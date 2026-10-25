package reward

import "sandboxctl/internal/sandbox"

const (
	Pass = 1.0
	Fail = 0.0
)

func FromExec(result sandbox.ExecResult) float64 {
	if result.TimedOut {
		return Fail
	}
	if result.ExitCode == 0 {
		return Pass
	}
	return Fail
}
