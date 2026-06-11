// Package state persists the "last stopped step" so `actdbg shell` can
// re-enter it later.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	Container  string            `json:"container"`
	JobID      string            `json:"job_id"`
	StepName   string            `json:"step_name"`
	StepNumber int               `json:"step_number"` // 1-based among Main steps
	TotalSteps int               `json:"total_steps"`
	Workdir    string            `json:"workdir"` // inside the container
	Env        map[string]string `json:"env"`     // reconstructed static env
	EnvNote    string            `json:"env_note"`
	Workflow   string            `json:"workflow"`
	SavedAt    time.Time         `json:"saved_at"`
}

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, ".actdbg")
	return d, os.MkdirAll(d, 0o700)
}

func Save(s *State) error {
	d, err := dir()
	if err != nil {
		return err
	}
	s.SavedAt = time.Now()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, "last.json"), b, 0o600)
}

func Load() (*State, error) {
	d, err := dir()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(d, "last.json"))
	if err != nil {
		return nil, fmt.Errorf("nothing saved yet")
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
