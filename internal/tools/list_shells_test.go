package tools

import (
	"context"
	"encoding/json"
	"testing"

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

	// Start some background shells
	_, err := state.executeBashCommand(context.Background(), "sleep 1", "First task", 0, true)
	require.NoError(t, err)

	_, err = state.executeBashCommand(context.Background(), "echo hello", "Second task", 0, true)
	require.NoError(t, err)

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

	// Check first shell
	assert.Equal(t, "shell_1", parsed.Shells[0].ID)
	assert.Equal(t, "First task", parsed.Shells[0].Description)
	assert.Contains(t, []string{"running", "completed"}, parsed.Shells[0].Status)

	// Check second shell
	assert.Equal(t, "shell_2", parsed.Shells[1].ID)
	assert.Equal(t, "Second task", parsed.Shells[1].Description)
	assert.Contains(t, []string{"running", "completed"}, parsed.Shells[1].Status)
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
	_, err := state.executeBashCommand(context.Background(), "sleep 1", "", 0, true)
	require.NoError(t, err)

	// List shells
	result, err := state.executeListShells(context.Background())
	require.NoError(t, err)

	var parsed listShellsResult
	err = json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err)

	assert.Equal(t, 1, parsed.Count)
	assert.Equal(t, "", parsed.Shells[0].Description)
}
