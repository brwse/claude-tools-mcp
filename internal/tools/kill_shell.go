package tools

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *State) executeKillShell(ctx context.Context, shellID string) (string, error) {
	if shellID == "" {
		return "", fmt.Errorf("shell_id is required.")
	}

	s.Mu.Lock()
	shell, exists := s.BackgroundShells[shellID]
	s.Mu.Unlock()

	if !exists {
		return "", fmt.Errorf("Background shell with ID '%s' not found.", shellID)
	}

	// Non-blocking check using select prevents attempting to kill a process that has already
	// completed. The background goroutine closes Done when cmd.Wait() returns, so we check this
	// first to avoid errors from killing a process that no longer exists and to provide proper
	// error messaging if the shell has already terminated.
	select {
	case <-shell.Done:
		return "", fmt.Errorf("Shell %s has already completed. Cannot kill a finished process.", shellID)
	default:
		// Guard against nil Process in edge cases where the cmd.Start() may not have completed
		// the process initialization, though this is rare in normal operation.
		if shell.Cmd.Process != nil {
			if err := shell.Cmd.Process.Kill(); err != nil {
				return "", fmt.Errorf("Failed to kill shell %s: %s", shellID, err)
			}
		}

		// Delay allows OS-level cleanup and ensures the process has begun termination before
		// removing the shell record from state. This prevents potential race conditions with the
		// background monitoring goroutine and ensures a clean shutdown sequence.
		time.Sleep(100 * time.Millisecond)

		s.Mu.Lock()
		delete(s.BackgroundShells, shellID)
		s.Mu.Unlock()

		return fmt.Sprintf("Successfully killed shell: %s (%s)", shellID, shell.Command), nil
	}
}

var KillShellTool = sdk.Tool{
	Name:        "kill_shell",
	Description: "- Kills a running background bash shell by its ID\n- Takes a shell_id parameter identifying the shell to kill\n- Returns a success or failure status \n- Use this tool when you need to terminate a long-running shell",
}

type KillShellInput struct {
	ShellID string `json:"shell_id" jsonschema:"The ID of the background shell to kill"`
}
type KillShellOutput struct {
	Message string `json:"message"`
}

func KillShell(ctx context.Context, req *sdk.CallToolRequest, args KillShellInput) (*sdk.CallToolResult, any, error) {
	server := GetState()
	result, err := server.executeKillShell(ctx, args.ShellID)
	if err != nil {
		return nil, nil, err
	}
	output := &KillShellOutput{Message: result}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: result}},
		StructuredContent: output,
	}, output, nil
}
