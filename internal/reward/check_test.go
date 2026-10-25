package reward

import (
	"testing"

	"sandboxctl/internal/sandbox"
)

func TestFromExec(t *testing.T) {
	if FromExec(sandbox.ExecResult{ExitCode: 0}) != Pass {
		t.Fatal("exit 0 should pass")
	}
	if FromExec(sandbox.ExecResult{ExitCode: 1}) != Fail {
		t.Fatal("nonzero should fail")
	}
	if FromExec(sandbox.ExecResult{ExitCode: 0, TimedOut: true}) != Fail {
		t.Fatal("timeout should fail")
	}
}
