package trajectory

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sandboxctl/internal/sandbox"
)

func TestAppendWritesOneObjectPerLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trajectory.jsonl")
	l, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	events := []sandbox.ExecResult{
		{Cmd: "echo hello > /tmp/hello.txt", Phase: "step", Stdout: "line1\nline2\n"},
		{Cmd: `printf '"q" \\ <tag> &'`, Phase: "step", Stdout: "\x1b[31mred\x1b[0m\r\n", ExitCode: 1},
		{Cmd: "grep -q hello /tmp/hello.txt", Phase: "success_check"},
	}
	for _, e := range events {
		if err := l.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var got []sandbox.ExecResult
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, "\\u003e") || strings.Contains(line, "\\u0026") {
			t.Errorf("line is HTML-escaped: %s", line)
		}
		var e sandbox.ExecResult
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("line %d is not valid JSON: %v: %s", len(got)+1, err, line)
		}
		got = append(got, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(events) {
		t.Fatalf("got %d lines, want %d", len(got), len(events))
	}
	for i := range events {
		if got[i] != events[i] {
			t.Errorf("event %d round-trip mismatch:\n got  %+v\n want %+v", i, got[i], events[i])
		}
	}
}
