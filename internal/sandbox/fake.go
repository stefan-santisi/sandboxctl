package sandbox

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Fake is an in-memory runtime for tests. Commands that contain "fail"
// exit 1; "sleep:" delays; "hang" waits on context.
type Fake struct {
	mu         sync.Mutex
	nextID     int
	Created    []string
	Execs      []string
	Snapshots  map[string]string
	TornDown   []string
	FailCreate bool
}

func NewFake() *Fake {
	return &Fake{Snapshots: map[string]string{}}
}

func (f *Fake) Create(ctx context.Context, image string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailCreate {
		return "", fmt.Errorf("create failed for %s", image)
	}
	f.nextID++
	id := fmt.Sprintf("fake-%d", f.nextID)
	f.Created = append(f.Created, image+"="+id)
	return id, nil
}

func (f *Fake) Exec(ctx context.Context, containerID, cmd string) (ExecResult, error) {
	start := time.Now()
	f.mu.Lock()
	f.Execs = append(f.Execs, containerID+":"+cmd)
	f.mu.Unlock()

	switch {
	case strings.Contains(cmd, "hang"):
		<-ctx.Done()
		return ExecResult{
			Cmd:        cmd,
			ExitCode:   -1,
			TimedOut:   ctx.Err() == context.DeadlineExceeded,
			Stderr:     ctx.Err().Error(),
			DurationMS: time.Since(start).Milliseconds(),
		}, nil
	case strings.HasPrefix(cmd, "sleep:"):
		n := strings.TrimPrefix(cmd, "sleep:")
		d, err := time.ParseDuration(n)
		if err != nil {
			d = 10 * time.Millisecond
		}
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ExecResult{
				Cmd:        cmd,
				ExitCode:   -1,
				TimedOut:   ctx.Err() == context.DeadlineExceeded,
				Stderr:     ctx.Err().Error(),
				DurationMS: time.Since(start).Milliseconds(),
			}, nil
		case <-timer.C:
		}
	}

	exit := 0
	if strings.Contains(cmd, "fail") {
		exit = 1
	}
	return ExecResult{
		Cmd:        cmd,
		Stdout:     "ok\n",
		ExitCode:   exit,
		DurationMS: time.Since(start).Milliseconds(),
	}, nil
}

func (f *Fake) Snapshot(ctx context.Context, containerID, tag string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Snapshots[containerID] = tag
	return nil
}

func (f *Fake) Teardown(ctx context.Context, containerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.TornDown = append(f.TornDown, containerID)
	return nil
}
