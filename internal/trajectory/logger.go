package trajectory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"sandboxctl/internal/sandbox"
)

type Logger struct {
	path string
	file *os.File
}

func Create(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create trajectory dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create trajectory file: %w", err)
	}
	return &Logger{path: path, file: f}, nil
}

func (l *Logger) Path() string {
	return l.path
}

func (l *Logger) Append(result sandbox.ExecResult) error {
	enc := json.NewEncoder(l.file)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("write trajectory event: %w", err)
	}
	return nil
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
