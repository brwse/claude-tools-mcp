package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type bashOutputResult struct {
	Status    string `json:"status"`
	ExitCode  int    `json:"exit_code,omitempty"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Timestamp string `json:"timestamp"`
}

func (s *State) executeBashOutput(ctx context.Context, shellID, filter string) (string, error) {
	if shellID == "" {
		return "", fmt.Errorf("bash_id is required.")
	}

	// Check shell existence with minimal lock duration before accessing its data.
	// We release early to avoid holding the lock during stdout/stderr reads on SyncBuffer.
	s.Mu.Lock()
	shell, exists := s.BackgroundShells[shellID]
	s.Mu.Unlock()
	if !exists {
		return "", fmt.Errorf("Background shell with ID '%s' not found.", shellID)
	}

	timestamp := time.Now().Format(time.RFC3339Nano)

	// Re-acquire lock for reading and updating the shell's output position markers.
	// This ensures thread-safe updates to LastStdoutReadAt and LastStderrReadAt.
	s.Mu.Lock()
	defer s.Mu.Unlock()

	// Extract only new output since the last read position.
	// These position markers ensure API consumers always see new data since their last call,
	// preventing duplicate output in streaming scenarios.
	stdoutContent := shell.Stdout.String()
	stderrContent := shell.Stderr.String()
	newStdout := stdoutContent[shell.LastStdoutReadAt:]
	newStderr := stderrContent[shell.LastStderrReadAt:]
	shell.LastStdoutReadAt = len(stdoutContent)
	shell.LastStderrReadAt = len(stderrContent)

	// Determine shell status without blocking on channel receive.
	// Non-blocking select returns "running" if Done channel is not yet closed.
	var exitCode int
	var statusStr string
	select {
	case <-shell.Done:
		exitCode = shell.ExitCode
		if shell.ExitCode != 0 {
			statusStr = "failed"
		} else {
			statusStr = "completed"
		}
	default:
		statusStr = "running"
	}

	// Apply regex filter only to new output if provided.
	// This allows callers to reduce output volume for long-running shells with verbose output.
	if filter != "" {
		filteredStdout, err := filterOutput(newStdout, filter)
		if err != nil {
			return "", err
		}
		filteredStderr, err := filterOutput(newStderr, filter)
		if err != nil {
			return "", err
		}
		newStdout = filteredStdout
		newStderr = filteredStderr
	}

	// Log output size for debugging; errors are ignored to ensure output is still returned
	// even if size checks fail. This supports monitoring of potentially excessive output.
	_ = checkOutputSize(ctx, newStdout, "bash")
	_ = checkOutputSize(ctx, newStderr, "bash")

	output := bashOutputResult{
		Status:    statusStr,
		ExitCode:  exitCode,
		Stdout:    newStdout,
		Stderr:    newStderr,
		Timestamp: timestamp,
	}
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("Failed to format output: %s", err)
	}
	return string(jsonBytes), nil
}

func filterOutput(output, pattern string) (string, error) {
	if pattern == "" {
		return output, nil
	}

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("Invalid filter regex: %s", err)
	}

	// Preserve trailing newline from input to maintain output formatting consistency.
	// This matters for tools that parse line-based output and expect final newlines.
	hasTrailingNewline := strings.HasSuffix(output, "\n")
	lines := strings.Split(output, "\n")
	var filtered []string
	for _, line := range lines {
		if regex.MatchString(line) {
			filtered = append(filtered, line)
		}
	}

	result := strings.Join(filtered, "\n")

	// Restore trailing newline only if output had one originally and filtering didn't
	// reduce result to empty. This prevents spurious newlines on empty filter results.
	if hasTrailingNewline && result != "" {
		result += "\n"
	}
	return result, nil
}

var BashOutputTool = sdk.Tool{
	Name:        "bash_output",
	Description: "- Retrieves output from a running or completed background bash shell\n- Takes a shell_id parameter identifying the shell\n- Always returns only new output since the last check\n- Returns stdout and stderr output along with shell status\n- Supports optional regex filtering to show only lines matching a pattern\n- Use this tool when you need to monitor or check the output of a long-running shell",
}

type BashOutputInput struct {
	ShellID string `json:"shell_id" jsonschema:"The ID of the background shell to retrieve output from"`
	Filter  string `json:"filter,omitempty" jsonschema:"Optional regular expression to filter the output lines. Only lines matching this regex will be included in the result. Any lines that do not match will no longer be available to read."`
}
type BashOutputOutput struct {
	Output string `json:"output"`
}

func BashOutput(ctx context.Context, req *sdk.CallToolRequest, args BashOutputInput) (*sdk.CallToolResult, any, error) {
	server := GetState()
	result, err := server.executeBashOutput(ctx, args.ShellID, args.Filter)
	if err != nil {
		return nil, nil, err
	}
	output := &BashOutputOutput{Output: result}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: result}},
		StructuredContent: output,
	}, output, nil
}
