package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func callBash(t *testing.T, state *State, input BashInput) (string, error) {
	t.Helper()
	return state.executeBashCommand(context.Background(), input.Command, input.Description, input.Timeout, input.RunInBackground)
}

// extractShellID parses the background shell ID from the command output.
// The output format is "Command running in background with ID: shell_N", and the
// ID may be followed by a comma in certain output formats, so we trim it.
func extractShellID(output string) string {
	fields := strings.FieldsSeq(output)
	for part := range fields {
		if strings.HasPrefix(part, "shell_") {
			return strings.TrimSuffix(part, ",")
		}
	}
	return ""
}

func TestBash_BasicFunctionality(t *testing.T) {
	state := NewState()
	t.Run("simple command", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			Command: "echo 'Hello, World!'",
		})
		require.NoError(t, err)
		assert.Equal(t, "Hello, World!\n", result)
	})
	t.Run("command with exit code", func(t *testing.T) {
		_, err := callBash(t, state, BashInput{
			Command: "exit 1",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exited with code 1")
	})
	t.Run("empty command rejected", func(t *testing.T) {
		_, err := callBash(t, state, BashInput{
			Command: "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be empty")
	})
}

func TestBash_Timeouts(t *testing.T) {
	state := NewState()
	t.Run("custom timeout success", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			Command: "sleep 0.1 && echo 'done'",
			Timeout: 1000,
		})
		require.NoError(t, err)
		assert.Equal(t, "done\n", result)
	})
	t.Run("timeout exceeded", func(t *testing.T) {
		_, err := callBash(t, state, BashInput{
			Command: "sleep 2",
			Timeout: 100,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
	})
	t.Run("timeout too large", func(t *testing.T) {
		_, err := callBash(t, state, BashInput{
			Command: "echo test",
			Timeout: 700000,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot exceed 600000")
	})
}

func TestBash_Background(t *testing.T) {
	state := NewState()
	t.Run("starts background task", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			Command:         "echo 'background task'",
			RunInBackground: true,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "Command running in background with ID:")
		shellID := extractShellID(result)
		require.NotEmpty(t, shellID)
		assert.True(t, strings.HasPrefix(shellID, "shell_"))
	})
	t.Run("background shells tracked", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			Command:         "sleep 0.1",
			RunInBackground: true,
		})
		require.NoError(t, err)
		shellID := extractShellID(result)
		require.NotEmpty(t, shellID)
		// Verify shell is registered in state before the goroutine that monitors
		// completion has a chance to run. Lock ensures consistent access to shared state.
		state.Mu.Lock()
		shell, exists := state.BackgroundShells[shellID]
		state.Mu.Unlock()
		require.True(t, exists)
		assert.Equal(t, "sleep 0.1", shell.Command)
		assert.NotNil(t, shell.Cmd)
	})
	t.Run("multiple background shells", func(t *testing.T) {
		shellIDs := make([]string, 5)
		for i := range 5 {
			result, err := callBash(t, state, BashInput{
				Command:         fmt.Sprintf("echo 'Shell %d' && sleep 0.2", i),
				RunInBackground: true,
			})
			require.NoError(t, err)
			shellIDs[i] = extractShellID(result)
		}
		// Ensure each shell receives a unique ID, which is critical for proper shell management
		// and preventing accidental operations on the wrong process.
		seen := make(map[string]bool)
		for _, id := range shellIDs {
			require.False(t, seen[id], "duplicate shell ID: %s", id)
			seen[id] = true
		}
		state.Mu.Lock()
		for _, id := range shellIDs {
			_, exists := state.BackgroundShells[id]
			assert.True(t, exists, "shell %s not found", id)
		}
		state.Mu.Unlock()
	})
}

func TestBashOutput(t *testing.T) {
	state := NewState()
	t.Run("reads output from completed shell", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			Command:         "echo 'test output' && sleep 0.1",
			RunInBackground: true,
		})
		require.NoError(t, err)
		shellID := extractShellID(result)
		require.NotEmpty(t, shellID)
		// Sleep to ensure the background goroutine has finished writing output
		// before we attempt to read it.
		time.Sleep(200 * time.Millisecond)
		output, err := state.executeBashOutput(context.Background(), shellID, "")
		require.NoError(t, err)
		assert.Contains(t, output, "test output")
	})
	t.Run("nonexistent shell error", func(t *testing.T) {
		_, err := state.executeBashOutput(context.Background(), "nonexistent_shell", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
	t.Run("empty shell_id error", func(t *testing.T) {
		_, err := state.executeBashOutput(context.Background(), "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bash_id is required")
	})
	t.Run("filter output by regex", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			Command:         "echo 'ERROR: something failed' && echo 'INFO: all good' && echo 'ERROR: another issue'",
			RunInBackground: true,
		})
		require.NoError(t, err)
		shellID := extractShellID(result)
		// Sleep ensures the shell completes execution before we query its output with filtering.
		// This tests that the filter regex is properly applied to the captured output.
		time.Sleep(200 * time.Millisecond)
		output, err := state.executeBashOutput(context.Background(), shellID, "ERROR:")
		require.NoError(t, err)
		assert.Contains(t, output, "ERROR: something failed")
		assert.Contains(t, output, "ERROR: another issue")
		assert.NotContains(t, output, "INFO: all good")
	})
	t.Run("invalid filter regex", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			Command:         "echo 'test'",
			RunInBackground: true,
		})
		require.NoError(t, err)
		shellID := extractShellID(result)
		_, err = state.executeBashOutput(context.Background(), shellID, "[invalid(regex")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid filter regex")
	})
}

func TestKillShell(t *testing.T) {
	state := NewState()
	t.Run("kills running shell", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			Command:         "sleep 10",
			RunInBackground: true,
		})
		require.NoError(t, err)
		shellID := extractShellID(result)
		// Brief sleep allows the shell to be properly registered before we kill it,
		// ensuring we test the happy path of killing an actual running process.
		time.Sleep(100 * time.Millisecond)
		killResult, err := state.executeKillShell(context.Background(), shellID)
		require.NoError(t, err)
		assert.Contains(t, killResult, "Successfully killed shell")
		assert.Contains(t, killResult, shellID)
		// Verify the shell is removed from tracking after being killed.
		_, err = state.executeBashOutput(context.Background(), shellID, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
	t.Run("nonexistent shell error", func(t *testing.T) {
		_, err := state.executeKillShell(context.Background(), "nonexistent_shell")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
	t.Run("empty shell_id error", func(t *testing.T) {
		_, err := state.executeKillShell(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shell_id is required")
	})
	t.Run("already completed shell", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			Command:         "echo 'done'",
			RunInBackground: true,
		})
		require.NoError(t, err)
		shellID := extractShellID(result)
		// Sleep longer than the command execution time to ensure it completes before
		// we attempt to kill it. This tests the error case of trying to kill a finished process.
		time.Sleep(200 * time.Millisecond)
		_, err = state.executeKillShell(context.Background(), shellID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already completed")
	})
}

func TestBash_MCPIntegration(t *testing.T) {
	t.Run("bash tool", func(t *testing.T) {
		result, _, err := Bash(context.Background(), &sdk.CallToolRequest{}, BashInput{
			Command: "echo 'test'",
		})
		require.NoError(t, err)
		assert.NotNil(t, result)
	})
	t.Run("bash output tool", func(t *testing.T) {
		result, _, err := Bash(context.Background(), &sdk.CallToolRequest{}, BashInput{
			Command:         "echo 'test'",
			RunInBackground: true,
		})
		require.NoError(t, err)
		// Type assertion is needed because result.Content is a slice of Content interface
		// types, and we must access the underlying TextContent to extract the shell ID.
		textContent := result.Content[0].(*sdk.TextContent)
		shellID := extractShellID(textContent.Text)
		time.Sleep(100 * time.Millisecond)
		output, _, err := BashOutput(context.Background(), &sdk.CallToolRequest{}, BashOutputInput{
			ShellID: shellID,
		})
		require.NoError(t, err)
		assert.NotNil(t, output)
	})
	t.Run("kill shell tool", func(t *testing.T) {
		result, _, err := Bash(context.Background(), &sdk.CallToolRequest{}, BashInput{
			Command:         "sleep 10",
			RunInBackground: true,
		})
		require.NoError(t, err)
		// Type assertion is needed because result.Content is a slice of Content interface
		// types, and we must access the underlying TextContent to extract the shell ID.
		textContent := result.Content[0].(*sdk.TextContent)
		shellID := extractShellID(textContent.Text)
		time.Sleep(100 * time.Millisecond)
		killResult, _, err := KillShell(context.Background(), &sdk.CallToolRequest{}, KillShellInput{
			ShellID: shellID,
		})
		require.NoError(t, err)
		assert.NotNil(t, killResult)
	})
}

func TestBash_EdgeCases(t *testing.T) {
	state := NewState()
	t.Run("large output", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			Command: "for i in {1..100}; do echo 'line'; done",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "line")
	})
	t.Run("special characters in command", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			// Bash metacharacters must be properly quoted in single quotes to prevent
			// shell interpretation. This tests that the command is passed as a literal string.
			Command: "echo 'test $VAR && || ; | > <'",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "$VAR")
	})
	t.Run("multiline command", func(t *testing.T) {
		result, err := callBash(t, state, BashInput{
			// Literal newlines in Go strings (via escape sequences) are passed to bash -c
			// and executed as separate commands. This tests the shell interpreter's ability
			// to handle multi-line input properly.
			Command: "echo 'line1'\necho 'line2'\necho 'line3'",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "line1")
		assert.Contains(t, result, "line2")
		assert.Contains(t, result, "line3")
	})
}
