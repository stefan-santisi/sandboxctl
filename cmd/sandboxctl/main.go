package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"sandboxctl/internal/list"
	"sandboxctl/internal/run"
	"sandboxctl/internal/sandbox"
	"sandboxctl/internal/task"
)

const usage = `sandboxctl — run scripted agent tasks in disposable containers.

Usage:
  sandboxctl run    -task FILE [options]
  sandboxctl list   [-results DIR]
  sandboxctl teardown CONTAINER_ID [CONTAINER_ID...]

Run options:
  -task FILE         Task spec JSON (repeatable)
  -results DIR       Results directory (default: ./results)
  -reset-from TAG    Start from a previously committed snapshot instead of task.image
  -repeat N          Repeat each task N times (default: 1)
  -parallel N        Worker pool size for repeated/multiple episodes (default: 1)
  -keep              Leave the container running after the episode
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var err error
	switch os.Args[1] {
	case "run":
		err = cmdRun(ctx, os.Args[2:])
	case "list":
		err = cmdList(os.Args[2:])
	case "teardown":
		err = cmdTeardown(ctx, os.Args[2:])
	case "-h", "-help", "--help", "help":
		fmt.Fprint(os.Stdout, usage)
		return
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandboxctl: %v\n", err)
		os.Exit(1)
	}
}

func cmdRun(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var tasks multiFlag
	results := fs.String("results", "results", "results directory")
	resetFrom := fs.String("reset-from", "", "snapshot tag to start from")
	repeat := fs.Int("repeat", 1, "repeat each task N times")
	parallel := fs.Int("parallel", 1, "number of concurrent episodes")
	keep := fs.Bool("keep", false, "do not teardown the container")
	fs.Var(&tasks, "task", "task spec JSON (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(tasks) == 0 {
		return fmt.Errorf("run: -task is required")
	}
	if *repeat < 1 {
		return fmt.Errorf("run: -repeat must be >= 1")
	}
	if *parallel < 1 {
		return fmt.Errorf("run: -parallel must be >= 1")
	}

	rt, err := sandbox.Detect()
	if err != nil {
		return err
	}

	var jobs []run.Job
	for _, path := range tasks {
		spec, err := task.Load(path)
		if err != nil {
			return err
		}
		for i := 0; i < *repeat; i++ {
			jobs = append(jobs, run.Job{
				Spec: spec,
				Opt: run.Options{
					ResultsDir: *results,
					ResetFrom:  *resetFrom,
					Keep:       *keep,
				},
			})
		}
	}

	resultsOut := run.Pool(ctx, rt, jobs, *parallel)
	failed := 0
	for _, r := range resultsOut {
		if r.Err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "episode %d (%s): %v\n", r.Index+1, r.Summary.Task, r.Err)
			continue
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r.Summary); err != nil {
			return err
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d episodes failed", failed, len(resultsOut))
	}
	return nil
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	results := fs.String("results", "results", "results directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	summaries, err := list.Summaries(*results)
	if err != nil {
		return err
	}
	if len(summaries) == 0 {
		fmt.Println("no runs found")
		return nil
	}
	return list.Print(summaries)
}

func cmdTeardown(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("teardown: container id required")
	}
	rt, err := sandbox.Detect()
	if err != nil {
		return err
	}
	var last error
	for _, id := range args {
		if err := rt.Teardown(ctx, id); err != nil {
			fmt.Fprintf(os.Stderr, "teardown %s: %v\n", id, err)
			last = err
		}
	}
	return last
}

type multiFlag []string

func (m *multiFlag) String() string { return fmt.Sprint(*m) }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
