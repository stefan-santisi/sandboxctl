package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ExecResult is one action inside the sandbox (one trajectory event).
type ExecResult struct {
	Cmd        string `json:"cmd"`
	Phase      string `json:"phase"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out,omitempty"`
}

// Runtime is the container lifecycle used by an episode.
type Runtime interface {
	Create(ctx context.Context, image string) (containerID string, err error)
	Exec(ctx context.Context, containerID, cmd string) (ExecResult, error)
	Snapshot(ctx context.Context, containerID, tag string) error
	Teardown(ctx context.Context, containerID string) error
}

type CLI struct {
	Binary string
}

func Detect() (*CLI, error) {
	if override := strings.TrimSpace(os.Getenv("SANDBOXCTL_RUNTIME")); override != "" {
		path, err := exec.LookPath(override)
		if err != nil {
			return nil, fmt.Errorf("SANDBOXCTL_RUNTIME=%s: %w", override, err)
		}
		return &CLI{Binary: path}, nil
	}
	for _, name := range []string{"podman", "docker"} {
		if path, err := exec.LookPath(name); err == nil {
			return &CLI{Binary: path}, nil
		}
	}
	return nil, errors.New("neither podman nor docker found in PATH (set SANDBOXCTL_RUNTIME to override)")
}

func (c *CLI) Create(ctx context.Context, image string) (string, error) {
	out, err := c.run(ctx, "run", "-d", "--rm=false", image, "sleep", "infinity")
	if err != nil {
		return "", fmt.Errorf("create container from %s: %w", image, err)
	}
	id := strings.TrimSpace(out)
	if id == "" {
		return "", fmt.Errorf("create container from %s: empty container id", image)
	}
	return id, nil
}

func (c *CLI) Exec(ctx context.Context, containerID, cmd string) (ExecResult, error) {
	result := ExecResult{Cmd: cmd, ExitCode: -1}
	start := time.Now()
	command := exec.CommandContext(ctx, c.Binary, "exec", containerID, "sh", "-c", cmd)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result.DurationMS = time.Since(start).Milliseconds()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	if err == nil {
		result.ExitCode = 0
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	if ctx.Err() != nil {
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		result.ExitCode = -1
		if result.Stderr == "" {
			result.Stderr = ctx.Err().Error()
		}
		return result, nil
	}
	return result, fmt.Errorf("exec %q: %w", cmd, err)
}

func (c *CLI) Snapshot(ctx context.Context, containerID, tag string) error {
	if _, err := c.run(ctx, "commit", containerID, tag); err != nil {
		return fmt.Errorf("snapshot %s as %s: %w", containerID, tag, err)
	}
	return nil
}

func (c *CLI) Teardown(ctx context.Context, containerID string) error {
	if _, err := c.run(ctx, "rm", "-f", containerID); err != nil {
		return fmt.Errorf("teardown %s: %w", containerID, err)
	}
	return nil
}

func (c *CLI) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s", c.Binary, strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
