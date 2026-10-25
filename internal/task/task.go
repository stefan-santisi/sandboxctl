package task

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Spec is one episode: initial state, actions, termination, and horizon.
type Spec struct {
	Name           string   `json:"name"`
	Image          string   `json:"image"`
	Setup          []string `json:"setup"`
	Steps          []string `json:"steps"`
	SuccessCheck   string   `json:"success_check"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

func Load(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, fmt.Errorf("read task spec: %w", err)
	}
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return Spec{}, fmt.Errorf("parse task spec %s: %w", path, err)
	}
	if err := spec.Validate(); err != nil {
		return Spec{}, fmt.Errorf("%s: %w", path, err)
	}
	return spec, nil
}

func (s Spec) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(s.Image) == "" {
		return fmt.Errorf("image is required")
	}
	if strings.TrimSpace(s.SuccessCheck) == "" {
		return fmt.Errorf("success_check is required")
	}
	if s.TimeoutSeconds < 0 {
		return fmt.Errorf("timeout_seconds must be >= 0")
	}
	return nil
}

func (s Spec) Horizon() time.Duration {
	if s.TimeoutSeconds <= 0 {
		return 0
	}
	return time.Duration(s.TimeoutSeconds) * time.Second
}
