# sandboxctl

A Go CLI that provisions a disposable container for one scripted AI-agent task, records every command as a trajectory, and emits a pass/fail reward. Same image in, isolated blast radius, comparable runs.

## Why

A **run** is one attempt at a bounded task inside a fresh environment: fix a broken repo, stand up Postgres, anything you can express as shell. The agent (or a replayed script) acts inside the box; you check whether it actually succeeded.

Containers keep a bad run from deleting the host or poisoning the next trial. Starting every episode from the same image is the agent equivalent of resetting fixtures between unit tests. Capturing the full command log — not just pass/fail — is what you need to debug a failure and the same `(actions, reward)` shape used for RL or eval suites like SWE-bench / TerminalBench.

## Build

Requires Go 1.22+ and either `podman` or `docker` on `PATH`. `SANDBOXCTL_RUNTIME` overrides the binary.

```bash
go build -o sandboxctl ./cmd/sandboxctl
```

Stdlib only. The runtime is a thin `os/exec` wrapper around the container CLI (`run` / `exec` / `commit` / `rm`), so the same code talks to Podman or Docker.

## Task spec

`examples/task.json`:

```json
{
  "name": "write-hello",
  "image": "docker.io/library/alpine:3.20",
  "setup": ["true"],
  "steps": ["echo hello > /tmp/hello.txt", "cat /tmp/hello.txt"],
  "success_check": "grep -q hello /tmp/hello.txt",
  "timeout_seconds": 60
}
```

That is a tiny MDP: `image` is initial state, `steps` are actions, `success_check` is the terminal reward (exit 0 → `1.0`, else `0.0`), `timeout_seconds` is the episode horizon.

## Commands

```bash
# One episode: create → setup → steps → success_check → snapshot → teardown
./sandboxctl run -task examples/task.json

# Past runs from ./results
./sandboxctl list

# Force-remove a container left behind with -keep
./sandboxctl teardown <container-id>
```

### Stretch flags

| Flag | Meaning |
|---|---|
| `-results DIR` | Where `summary.json` + `trajectory.jsonl` are written (default `./results`) |
| `-reset-from TAG` | `Create` from a previous `podman commit` snapshot instead of `task.image` |
| `-repeat N` | Replay the same spec N times |
| `-parallel N` | Worker pool across repeated / multiple `-task` files |
| `-keep` | Skip teardown (pair with `teardown`) |

Each `Exec` is wrapped in `context.WithTimeout` using remaining episode time. Horizon expiry records `timed_out`, skips leftover steps, reward `0.0`, then still snapshots and tears down.

## Output

```
results/<timestamp>-<task>/
  trajectory.jsonl   # one JSON object per command
  summary.json       # task, reward, duration, snapshot tag, trajectory path
```

Trajectory line:

```json
{"cmd":"echo hello > /tmp/hello.txt","phase":"step","stdout":"","stderr":"","exit_code":0,"duration_ms":12}
```

## Tests

```bash
go test ./...
```

Unit tests use an in-memory fake runtime (no daemon required).
