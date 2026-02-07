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
	// Per-repo sync tracking for throttling
	LastRepoSync        map[string]time.Time `json:"lastRepoSync,omitempty"`
	NextRepoSyncAllowed map[string]time.Time `json:"nextRepoSyncAllowed,omitempty"`
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

// EnsureRepoMaps initializes per-repo maps if needed
func (s *State) EnsureRepoMaps() {
	if s.LastRepoSync == nil {
		s.LastRepoSync = make(map[string]time.Time)
	}
	if s.NextRepoSyncAllowed == nil {
		s.NextRepoSyncAllowed = make(map[string]time.Time)
	}
}

// GetLastRepoSync returns the last sync time for a repo
func (s *State) GetLastRepoSync(repoName string) time.Time {
	if s == nil || s.LastRepoSync == nil {
		return time.Time{}
	}
	return s.LastRepoSync[repoName]
}

// GetNextRepoSyncAllowed returns the next allowed sync time for a repo
func (s *State) GetNextRepoSyncAllowed(repoName string) time.Time {
	if s == nil || s.NextRepoSyncAllowed == nil {
		return time.Time{}
	}
	return s.NextRepoSyncAllowed[repoName]
}

// SetRepoSync updates the last sync time and next allowed sync time for a repo
func (s *State) SetRepoSync(repoName string, lastSync time.Time, nextAllowed time.Time) {
	if s == nil {
		return
	}
	s.EnsureRepoMaps()
	s.LastRepoSync[repoName] = lastSync
	s.NextRepoSyncAllowed[repoName] = nextAllowed
}

// SetNextRepoSyncAllowed updates only the next allowed sync time for a repo
func (s *State) SetNextRepoSyncAllowed(repoName string, nextAllowed time.Time) {
	if s == nil {
		return
	}
	s.EnsureRepoMaps()
	s.NextRepoSyncAllowed[repoName] = nextAllowed
}
