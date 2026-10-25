package run

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sandboxctl/internal/sandbox"
	"sandboxctl/internal/task"
)

func TestEpisodePassAndTeardown(t *testing.T) {
	rt := sandbox.NewFake()
	spec := task.Spec{
		Name:           "ok-task",
		Image:          "alpine:test",
		Setup:          []string{"true"},
		Steps:          []string{"echo hello"},
		SuccessCheck:   "true",
		TimeoutSeconds: 5,
	}
	sum, err := Episode(context.Background(), rt, spec, Options{ResultsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Reward != 1.0 {
		t.Fatalf("reward=%v want 1.0", sum.Reward)
	}
	if len(rt.TornDown) != 1 {
		t.Fatalf("expected teardown, got %v", rt.TornDown)
	}
	if sum.Snapshot == "" {
		t.Fatal("expected snapshot tag")
	}
	if _, err := os.Stat(sum.Trajectory); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(sum.Trajectory)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var n int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var ev sandbox.ExecResult
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("trajectory events=%d want 3", n)
	}
}

func TestEpisodeFailingCheck(t *testing.T) {
	rt := sandbox.NewFake()
	spec := task.Spec{
		Name:         "fail-task",
		Image:        "alpine:test",
		Steps:        []string{"echo hello"},
		SuccessCheck: "fail now",
	}
	sum, err := Episode(context.Background(), rt, spec, Options{ResultsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Reward != 0.0 {
		t.Fatalf("reward=%v want 0.0", sum.Reward)
	}
}

func TestEpisodeTimeout(t *testing.T) {
	rt := sandbox.NewFake()
	spec := task.Spec{
		Name:           "slow",
		Image:          "alpine:test",
		Steps:          []string{"hang"},
		SuccessCheck:   "true",
		TimeoutSeconds: 1,
	}
	start := time.Now()
	sum, err := Episode(context.Background(), rt, spec, Options{ResultsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout did not fire promptly")
	}
	if !sum.TimedOut || sum.Reward != 0 {
		t.Fatalf("summary=%+v", sum)
	}
}

func TestResetFrom(t *testing.T) {
	rt := sandbox.NewFake()
	spec := task.Spec{
		Name:         "replay",
		Image:        "alpine:test",
		SuccessCheck: "true",
	}
	_, err := Episode(context.Background(), rt, spec, Options{
		ResultsDir: t.TempDir(),
		ResetFrom:  "localhost/sandboxctl-replay:prior",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.Created) != 1 || rt.Created[0] != "localhost/sandboxctl-replay:prior=fake-1" {
		t.Fatalf("created=%v", rt.Created)
	}
}

func TestPoolParallel(t *testing.T) {
	rt := sandbox.NewFake()
	spec := task.Spec{Name: "p", Image: "alpine:test", SuccessCheck: "true"}
	opt := Options{ResultsDir: t.TempDir()}
	jobs := []Job{{Spec: spec, Opt: opt}, {Spec: spec, Opt: opt}, {Spec: spec, Opt: opt}}
	out := Pool(context.Background(), rt, jobs, 3)
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}
	dirs := map[string]struct{}{}
	for _, r := range out {
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		if r.Summary.Reward != 1 {
			t.Fatalf("reward=%v", r.Summary.Reward)
		}
		dirs[r.Summary.RunDir] = struct{}{}
	}
	if len(dirs) != 3 {
		t.Fatalf("expected unique run dirs, got %v", dirs)
	}
}

func TestKeepSkipsTeardown(t *testing.T) {
	rt := sandbox.NewFake()
	spec := task.Spec{Name: "keep", Image: "alpine:test", SuccessCheck: "true"}
	_, err := Episode(context.Background(), rt, spec, Options{ResultsDir: t.TempDir(), Keep: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.TornDown) != 0 {
		t.Fatalf("teardown should be skipped: %v", rt.TornDown)
	}
}

func TestSummaryFile(t *testing.T) {
	rt := sandbox.NewFake()
	spec := task.Spec{Name: "sum", Image: "alpine:test", SuccessCheck: "true"}
	sum, err := Episode(context.Background(), rt, spec, Options{ResultsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(sum.RunDir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got Summary
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Task != "sum" || got.Trajectory == "" {
		t.Fatalf("summary file: %+v", got)
	}
}
