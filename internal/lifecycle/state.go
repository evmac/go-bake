package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// State is the contents of .bake/state.json (daemon PIDs for bake down).
type State struct {
	Daemons map[string]DaemonEntry `json:"daemons"`
}

// DaemonEntry records a running daemon: either a host process (PID) or a container (ContainerID).
// For host daemons PID is set and ContainerID is empty; for container daemons (when daemon has image) ContainerID is set and PID is 0.
type DaemonEntry struct {
	PID         int    `json:"pid"`
	ContainerID string `json:"container_id,omitempty"`
}

// StatePath returns the path to the state file under rootDir (.bake/state.json).
func StatePath(rootDir string) string {
	return filepath.Join(rootDir, ".bake", "state.json")
}

// Load reads the state file; returns nil state and nil error if file does not exist.
func Load(rootDir string) (*State, error) {
	p := StatePath(rootDir)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{Daemons: make(map[string]DaemonEntry)}, nil
		}
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if s.Daemons == nil {
		s.Daemons = make(map[string]DaemonEntry)
	}
	return &s, nil
}

// Save writes the state file, creating .bake if needed.
func Save(rootDir string, s *State) error {
	if s == nil {
		s = &State{Daemons: make(map[string]DaemonEntry)}
	}
	dir := filepath.Join(rootDir, ".bake")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StatePath(rootDir), b, 0644)
}

// AddDaemon records a daemon in state and saves. For host daemons use pid and leave containerID empty;
// for container daemons use containerID and pass pid 0.
func AddDaemon(rootDir, name string, pid int, containerID string) error {
	s, err := Load(rootDir)
	if err != nil {
		return err
	}
	s.Daemons[name] = DaemonEntry{PID: pid, ContainerID: containerID}
	return Save(rootDir, s)
}

// RemoveDaemon removes a daemon from state and saves.
func RemoveDaemon(rootDir, name string) error {
	s, err := Load(rootDir)
	if err != nil {
		return err
	}
	delete(s.Daemons, name)
	return Save(rootDir, s)
}
