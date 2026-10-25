package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sandboxctl/internal/reward"
	"sandboxctl/internal/sandbox"
	"sandboxctl/internal/task"
	"sandboxctl/internal/trajectory"
)

type Options struct {
	ResultsDir string
	ResetFrom  string
	Keep       bool
}

type Summary struct {
	Task        string  `json:"task"`
	Reward      float64 `json:"reward"`
	DurationMS  int64   `json:"duration_ms"`
	Trajectory  string  `json:"trajectory"`
	Snapshot    string  `json:"snapshot,omitempty"`
	ContainerID string  `json:"container_id,omitempty"`
	Image       string  `json:"image"`
	TimedOut    bool    `json:"timed_out,omitempty"`
	Error       string  `json:"error,omitempty"`
	StartedAt   string  `json:"started_at"`
	FinishedAt  string  `json:"finished_at"`
	RunDir      string  `json:"run_dir"`
}

func Episode(parent context.Context, rt sandbox.Runtime, spec task.Spec, opt Options) (Summary, error) {
	started := time.Now().UTC()
	runID := started.Format("20060102T150405Z") + "-" + sanitize(spec.Name)
	runDir := uniqueDir(filepath.Join(opt.ResultsDir, runID))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return Summary{}, fmt.Errorf("create results dir: %w", err)
	}

	log, err := trajectory.Create(filepath.Join(runDir, "trajectory.jsonl"))
	if err != nil {
		return Summary{}, err
	}
	defer log.Close()

	summary := Summary{
		Task:       spec.Name,
		Image:      spec.Image,
		Trajectory: log.Path(),
		StartedAt:  started.Format(time.RFC3339),
		RunDir:     runDir,
		Reward:     reward.Fail,
	}

	ctx := parent
	cancel := func() {}
	if horizon := spec.Horizon(); horizon > 0 {
		ctx, cancel = context.WithTimeout(parent, horizon)
	}
	defer cancel()

	image := spec.Image
	if opt.ResetFrom != "" {
		image = opt.ResetFrom
		summary.Image = image
	}

	id, err := rt.Create(ctx, image)
	if err != nil {
		summary.Error = err.Error()
		summary.TimedOut = ctx.Err() == context.DeadlineExceeded
		return finish(summary, started, runDir)
	}
	summary.ContainerID = id
	if !opt.Keep {
		defer func() {
			_ = rt.Teardown(context.Background(), id)
		}()
	}

	phases := []struct {
		name string
		cmds []string
	}{
		{"setup", spec.Setup},
		{"step", spec.Steps},
	}

	timedOut := false
	for _, phase := range phases {
		for _, cmd := range phase.cmds {
			result, execErr := execOne(ctx, rt, id, cmd)
			result.Phase = phase.name
			if appendErr := log.Append(result); appendErr != nil {
				summary.Error = appendErr.Error()
				return finish(summary, started, runDir)
			}
			if execErr != nil {
				summary.Error = execErr.Error()
				return snapshotAndFinish(ctx, rt, spec, opt, summary, started, runDir)
			}
			if result.TimedOut || ctx.Err() == context.DeadlineExceeded {
				timedOut = true
				break
			}
		}
		if timedOut {
			break
		}
	}

	check := sandbox.ExecResult{Cmd: spec.SuccessCheck, Phase: "success_check", ExitCode: -1}
	if !timedOut && ctx.Err() == nil {
		var execErr error
		check, execErr = execOne(ctx, rt, id, spec.SuccessCheck)
		check.Phase = "success_check"
		if execErr != nil {
			if appendErr := log.Append(check); appendErr != nil {
				summary.Error = appendErr.Error()
				return finish(summary, started, runDir)
			}
			summary.Error = execErr.Error()
			return snapshotAndFinish(ctx, rt, spec, opt, summary, started, runDir)
		}
		if check.TimedOut {
			timedOut = true
		}
	} else {
		timedOut = true
		check.TimedOut = true
		check.Stderr = "episode horizon reached before success_check"
	}
	if err := log.Append(check); err != nil {
		summary.Error = err.Error()
		return finish(summary, started, runDir)
	}

	summary.Reward = reward.FromExec(check)
	summary.TimedOut = timedOut
	return snapshotAndFinish(ctx, rt, spec, opt, summary, started, runDir)
}

func execOne(ctx context.Context, rt sandbox.Runtime, id, cmd string) (sandbox.ExecResult, error) {
	return rt.Exec(ctx, id, cmd)
}

func snapshotAndFinish(ctx context.Context, rt sandbox.Runtime, spec task.Spec, opt Options, summary Summary, started time.Time, runDir string) (Summary, error) {
	tag := fmt.Sprintf("localhost/sandboxctl-%s:%s", sanitize(spec.Name), filepath.Base(runDir))
	snapCtx := ctx
	if ctx.Err() != nil {
		snapCtx = context.Background()
	}
	if err := rt.Snapshot(snapCtx, summary.ContainerID, tag); err != nil {
		if summary.Error == "" {
			summary.Error = err.Error()
		}
	} else {
		summary.Snapshot = tag
	}
	return finish(summary, started, runDir)
}

func finish(summary Summary, started time.Time, runDir string) (Summary, error) {
	finished := time.Now().UTC()
	summary.FinishedAt = finished.Format(time.RFC3339)
	summary.DurationMS = finished.Sub(started).Milliseconds()

	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return summary, err
	}
	path := filepath.Join(runDir, "summary.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return summary, fmt.Errorf("write summary: %w", err)
	}

	if summary.Error != "" {
		return summary, fmt.Errorf("%s", summary.Error)
	}
	return summary, nil
}

var runSeq atomic.Uint64

func uniqueDir(base string) string {
	n := runSeq.Add(1)
	if n == 1 {
		if _, err := os.Stat(base); os.IsNotExist(err) {
			return base
		}
	}
	return fmt.Sprintf("%s-%d", base, n)
}

var unsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitize(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = unsafe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "task"
	}
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

type Job struct {
	Spec task.Spec
	Opt  Options
}

func Pool(parent context.Context, rt sandbox.Runtime, jobs []Job, workers int) []Result {
	if len(jobs) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}

	indexed := make(chan indexedJob)
	out := make([]Result, len(jobs))
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for item := range indexed {
				sum, err := Episode(parent, rt, item.job.Spec, item.job.Opt)
				out[item.i] = Result{Index: item.i, Summary: sum, Err: err}
			}
		}()
	}
	for i, job := range jobs {
		indexed <- indexedJob{i: i, job: job}
	}
	close(indexed)
	wg.Wait()
	return out
}

type indexedJob struct {
	i   int
	job Job
}

type Result struct {
	Index   int
	Summary Summary
	Err     error
}
