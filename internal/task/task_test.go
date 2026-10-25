package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAndValidate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.json")
	if err := os.WriteFile(path, []byte(`{
		"name": "demo",
		"image": "alpine:3.20",
		"setup": ["true"],
		"steps": ["echo hi"],
		"success_check": "true",
		"timeout_seconds": 5
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "demo" || spec.Horizon() != 5*time.Second {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestLoadRejectsMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}
