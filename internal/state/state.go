package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State represents the persistent state of gitsyncer
type State struct {
	LastBatchRun time.Time `json:"lastBatchRun"`
}

// Manager handles state persistence
type Manager struct {
	filePath string
}

// NewManager creates a new state manager
func NewManager(workDir string) *Manager {
	return &Manager{
		filePath: filepath.Join(workDir, ".gitsyncer-state.json"),
	}
}

// Load reads the state from disk
func (m *Manager) Load() (*State, error) {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty state if file doesn't exist
			return &State{}, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return &state, nil
}

// Save writes the state to disk
func (m *Manager) Save(state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Write state file
	if err := os.WriteFile(m.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// HasRunWithinWeek checks if the last batch run was within the past week
func (s *State) HasRunWithinWeek() bool {
	if s.LastBatchRun.IsZero() {
		return false
	}
	return time.Since(s.LastBatchRun) < 7*24*time.Hour
}

// UpdateBatchRunTime updates the last batch run timestamp to now
func (s *State) UpdateBatchRunTime() {
	s.LastBatchRun = time.Now()
}
