package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListShells_NoShells(t *testing.T) {
	state := NewState()
	result, err := state.executeListShells(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "No background shells are currently running.", result)
}

func TestListShells_WithShells(t *testing.T) {
	state := NewState()

	// Start some background shells with sleep to ensure different timestamps
	_, err := state.executeBashCommand(context.Background(), "sleep 10", "First task", 0, true)
	require.NoError(t, err)

	// Delay to ensure different Unix timestamps (second precision) for deterministic ordering
	time.Sleep(1 * time.Second)

	_, err = state.executeBashCommand(context.Background(), "sleep 10", "Second task", 0, true)
	require.NoError(t, err)

	// Clean up background shells after test
	defer func() {
		state.Mu.Lock()
		for _, shell := range state.BackgroundShells {
			if shell.Cmd != nil && shell.Cmd.Process != nil {
				_ = shell.Cmd.Process.Kill()
			}
		}
		state.Mu.Unlock()
	}()

	// List shells
	result, err := state.executeListShells(context.Background())
	require.NoError(t, err)

	// Parse JSON result
	var parsed listShellsResult
	err = json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err)

	// Verify result
	assert.Equal(t, 2, parsed.Count)
	assert.Len(t, parsed.Shells, 2)

	// Check first shell - should be running with long sleep command
	assert.Equal(t, "shell_1", parsed.Shells[0].ID)
	assert.Equal(t, "First task", parsed.Shells[0].Description)
	assert.Equal(t, "running", parsed.Shells[0].Status)

	// Check second shell - should be running with long sleep command
	assert.Equal(t, "shell_2", parsed.Shells[1].ID)
	assert.Equal(t, "Second task", parsed.Shells[1].Description)
	assert.Equal(t, "running", parsed.Shells[1].Status)
}

func TestListShells_StatusTransitions(t *testing.T) {
	state := NewState()

	// Start a quick command that will complete
	_, err := state.executeBashCommand(context.Background(), "echo test", "Quick task", 0, true)
	require.NoError(t, err)

	// Wait for completion
	state.Mu.RLock()
	shell := state.BackgroundShells["shell_1"]
	state.Mu.RUnlock()
	<-shell.Done

	// List shells and verify status is "completed"
	result, err := state.executeListShells(context.Background())
	require.NoError(t, err)

	var parsed listShellsResult
	err = json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 1, parsed.Count)
	assert.Equal(t, "completed", parsed.Shells[0].Status)
}

func TestListShells_FailedCommand(t *testing.T) {
	state := NewState()

	// Start a command that will fail
	_, err := state.executeBashCommand(context.Background(), "exit 1", "Failing task", 0, true)
	require.NoError(t, err)

	// Wait for completion
	state.Mu.RLock()
	shell := state.BackgroundShells["shell_1"]
	state.Mu.RUnlock()
	<-shell.Done

	// List shells and verify status is "failed"
	result, err := state.executeListShells(context.Background())
	require.NoError(t, err)

	var parsed listShellsResult
	err = json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 1, parsed.Count)
	assert.Equal(t, "failed", parsed.Shells[0].Status)
}

func TestListShells_EmptyDescription(t *testing.T) {
	state := NewState()

	// Start a shell without description
	_, err := state.executeBashCommand(context.Background(), "sleep 10", "", 0, true)
	require.NoError(t, err)

	// Clean up background shell after test
	defer func() {
		state.Mu.Lock()
		for _, shell := range state.BackgroundShells {
			if shell.Cmd != nil && shell.Cmd.Process != nil {
				_ = shell.Cmd.Process.Kill()
			}
		}
		state.Mu.Unlock()
	}()

	// List shells
	result, err := state.executeListShells(context.Background())
	require.NoError(t, err)

	var parsed listShellsResult
	err = json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 1, parsed.Count)
	assert.Equal(t, "", parsed.Shells[0].Description)
	assert.Equal(t, "running", parsed.Shells[0].Status)
}
