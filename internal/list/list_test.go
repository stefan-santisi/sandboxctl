package list

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sandboxctl/internal/run"
)

func TestSummariesSorted(t *testing.T) {
	dir := t.TempDir()
	write := func(subdir string, s run.Summary) {
		p := filepath.Join(dir, subdir)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(s)
		if err := os.WriteFile(filepath.Join(p, "summary.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a", run.Summary{Task: "old", StartedAt: "2026-01-01T00:00:00Z", Reward: 0})
	write("b", run.Summary{Task: "new", StartedAt: "2026-02-01T00:00:00Z", Reward: 1})
	got, err := Summaries(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Task != "new" || got[1].Task != "old" {
		t.Fatalf("got=%+v", got)
	}
}

func TestSummariesMissingDir(t *testing.T) {
	got, err := Summaries(filepath.Join(t.TempDir(), "missing"))
	if err != nil || got != nil {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
