package tools

import (
	"sync"
	"time"
)

// State manages global application state for the tools package, including
// file access tracking and background shell processes. Access to State is
// synchronized via its embedded RWMutex to support concurrent read/write
// operations from multiple tool handlers.
type State struct {
	Mu sync.RWMutex

	// ReadFiles tracks the modification times of files that have been read,
	// used to detect when file content may have changed between operations.
	ReadFiles map[string]time.Time

	// BackgroundShells maps shell IDs to their corresponding BackgroundShell
	// structs, allowing callers to monitor running processes and retrieve output.
	BackgroundShells map[string]*BackgroundShell

	// NextShellID is a monotonically increasing counter used to generate unique
	// shell IDs (e.g., "shell_1", "shell_2"). Must be incremented atomically
	// when protected by Mu.Lock() to ensure IDs remain globally unique.
	NextShellID int
}

// globalState is the singleton instance of State for the entire tools package.
// It is initialized once at package load time and accessed via GetState() to
// provide a consistent entry point for all state management operations.
var globalState *State

func init() {
	globalState = NewState()
}

func NewState() *State {
	return &State{
		ReadFiles:        make(map[string]time.Time),
		BackgroundShells: make(map[string]*BackgroundShell),
		NextShellID:      1,
	}
}

// GetState returns the global State singleton for the tools package.
func GetState() *State {
	return globalState
}
