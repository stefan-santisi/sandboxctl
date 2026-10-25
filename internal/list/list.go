package list

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"sandboxctl/internal/run"
)

func Summaries(resultsDir string) ([]run.Summary, error) {
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read results dir: %w", err)
	}

	var out []run.Summary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(resultsDir, entry.Name(), "summary.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s run.Summary
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		if s.RunDir == "" {
			s.RunDir = filepath.Join(resultsDir, entry.Name())
		}
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt > out[j].StartedAt
	})
	return out, nil
}

func Print(summaries []run.Summary) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "STARTED\tTASK\tREWARD\tDURATION\tTIMEOUT\tSNAPSHOT\tDIR")
	for _, s := range summaries {
		started := s.StartedAt
		if t, err := time.Parse(time.RFC3339, s.StartedAt); err == nil {
			started = t.Local().Format("2006-01-02 15:04:05")
		}
		timeout := ""
		if s.TimedOut {
			timeout = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%.1f\t%dms\t%s\t%s\t%s\n",
			started,
			s.Task,
			s.Reward,
			s.DurationMS,
			timeout,
			dash(s.Snapshot),
			s.RunDir,
		)
	}
	return w.Flush()
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
