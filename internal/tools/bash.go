package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// defaultTimeout is 2 minutes - chosen to prevent long-running operations
	// from blocking foreground execution while still allowing reasonable time
	// for most shell operations (git, npm, docker, etc.)
	defaultTimeout = 120000
	// maxTimeout is 10 minutes - enforced to prevent indefinite blocking
	// and memory exhaustion from extremely long-running processes
	maxTimeout = 600000
)

// BackgroundShell represents a long-running command executing asynchronously.
// LastStdoutReadAt and LastStderrReadAt track byte positions to support
// fetching only new output on subsequent reads, avoiding re-transmission
// of already-returned data and respecting size constraints.
type BackgroundShell struct {
	ID               string
	Command          string
	Cmd              *exec.Cmd
	Stdout           *SyncBuffer
	Stderr           *SyncBuffer
	StartTime        time.Time
	Done             chan struct{}
	Err              error
	ExitCode         int
	LastStdoutReadAt int
	LastStderrReadAt int
}

func (s *State) executeBashCommand(ctx context.Context, command, description string, timeout int64, runInBackground bool) (string, error) {
	if command == "" {
		return "", fmt.Errorf("Command cannot be empty.")
	}

	timeoutMs := defaultTimeout
	if timeout > 0 {
		if timeout > maxTimeout {
			return "", fmt.Errorf("Timeout cannot exceed %d milliseconds (10 minutes).", maxTimeout)
		}
		timeoutMs = int(timeout)
	}

	// Background commands don't use context timeout because they run asynchronously
	// and their output is retrieved later via BashOutput. Foreground commands use
	// context timeout to enforce synchronous execution limits.
	var cmd *exec.Cmd
	if runInBackground {
		cmd = exec.Command("bash", "-c", command)
	} else {
		cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
		cmd = exec.CommandContext(cmdCtx, "bash", "-c", command)
	}

	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}

	if runInBackground {
		return s.executeBackground(cmd, command)
	}
	return s.executeForeground(ctx, cmd, command)
}

func (s *State) executeForeground(ctx context.Context, cmd *exec.Cmd, command string) (string, error) {
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return "", fmt.Errorf("Command timed out. Consider increasing the timeout parameter or running in background.")
		}

		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode := exitErr.ExitCode()
			// On Unix/Linux, a killed process (e.g., by timeout signal) returns exit code -1
			// rather than the actual signal number. Detect this to provide clearer error messaging.
			if exitCode == -1 && strings.Contains(err.Error(), "signal: killed") {
				return "", fmt.Errorf("Command timed out. Consider increasing the timeout parameter or running in background.")
			}

			return "", fmt.Errorf(
				"Command exited with code %d:\n%s\n\nCommand: %s",
				exitCode,
				string(output),
				command,
			)
		}

		return "", fmt.Errorf("Failed to execute command: %s\n\nCommand: %s", err, command)
	}

	result := string(output)
	if err := checkOutputSize(ctx, result, "bash"); err != nil {
		return "", err
	}

	return result, nil
}

func (s *State) executeBackground(cmd *exec.Cmd, command string) (string, error) {
	// SyncBuffer is needed because both the subprocess and the BashOutput
	// goroutine will read from stdout/stderr concurrently
	stdout := &SyncBuffer{}
	stderr := &SyncBuffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("Failed to start background command: %s", err)
	}

	s.Mu.Lock()
	shellID := fmt.Sprintf("shell_%d", s.NextShellID)
	s.NextShellID++
	shell := &BackgroundShell{
		ID:        shellID,
		Command:   command,
		Cmd:       cmd,
		Stdout:    stdout,
		Stderr:    stderr,
		StartTime: time.Now(),
		Done:      make(chan struct{}),
	}
	s.BackgroundShells[shellID] = shell
	s.Mu.Unlock()

	// Monitor process completion in a separate goroutine to avoid blocking
	// and to capture exit code/error for later retrieval
	go func() {
		err := cmd.Wait()
		s.Mu.Lock()
		defer s.Mu.Unlock()
		shell.Err = err
		if cmd.ProcessState != nil {
			shell.ExitCode = cmd.ProcessState.ExitCode()
		}
		close(shell.Done)
	}()

	return fmt.Sprintf("Command running in background with ID: %s", shellID), nil
}

// SyncBuffer wraps bytes.Buffer with a mutex to allow safe concurrent reads
// from both the subprocess (writing output) and the BashOutput handler
// (reading output). This is essential because the process writes continuously
// while callers may read asynchronously.
type SyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (sb *SyncBuffer) Write(p []byte) (n int, err error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.Write(p)
}

func (sb *SyncBuffer) String() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return sb.buf.String()
}

var (
	_ io.Writer = (*SyncBuffer)(nil)

	BashTool = sdk.Tool{
		Name:        "bash",
		Description: "Executes a given bash command in a persistent shell session with optional timeout, ensuring proper handling and security measures.\n\nIMPORTANT: This tool is for terminal operations like git, npm, docker, etc. DO NOT use it for file operations (reading, writing, editing, searching, finding files) - use the specialized tools for this instead.\n\nBefore executing the command, please follow these steps:\n\n1. Directory Verification:\n   - If the command will create new directories or files, first use `ls` to verify the parent directory exists and is the correct location\n   - For example, before running \"mkdir foo/bar\", first use `ls foo` to check that \"foo\" exists and is the intended parent directory\n\n2. Command Execution:\n   - Always quote file paths that contain spaces with double quotes (e.g., cd \"path with spaces/file.txt\")\n   - Examples of proper quoting:\n     - cd \"/Users/name/My Documents\" (correct)\n     - cd /Users/name/My Documents (incorrect - will fail)\n     - python \"/path/with spaces/script.py\" (correct)\n     - python /path/with spaces/script.py (incorrect - will fail)\n   - After ensuring proper quoting, execute the command.\n   - Capture the output of the command.\n\nUsage notes:\n  - The command argument is required.\n  - You can specify an optional timeout in milliseconds (up to 600000ms / 10 minutes). If not specified, commands will timeout after 120000ms (2 minutes).\n  - It is very helpful if you write a clear, concise description of what this command does in 5-10 words.\n  - You can use the `run_in_background` parameter to run the command in the background, which allows you to continue working while the command runs. You can monitor the output using the Bash tool as it becomes available. Never use `run_in_background` to run 'sleep' as it will return immediately. You do not need to use '&' at the end of the command when using this parameter.\n  \n  - Avoid using Bash with the `find`, `grep`, `cat`, `head`, `tail`, `sed`, `awk`, or `echo` commands, unless explicitly instructed or when these commands are truly necessary for the task. Instead, always prefer using the dedicated tools for these commands:\n    - File search: Use Glob (NOT find or ls)\n    - Content search: Use Grep (NOT grep or rg)\n    - Read files: Use Read (NOT cat/head/tail)\n    - Edit files: Use Edit (NOT sed/awk)\n    - Write files: Use Write (NOT echo >/cat <<EOF)\n    - Communication: Output text directly (NOT echo/printf)\n  - When issuing multiple commands:\n    - If the commands are independent and can run in parallel, make multiple Bash tool calls in a single message. For example, if you need to run \"git status\" and \"git diff\", send a single message with two Bash tool calls in parallel.\n    - If the commands depend on each other and must run sequentially, use a single Bash call with '&&' to chain them together (e.g., `git add . && git commit -m \"message\" && git push`). For instance, if one operation must complete before another starts (like mkdir before cp, Write before Bash for git operations, or git add before git commit), run these operations sequentially instead.\n    - Use ';' only when you need to run commands sequentially but don't care if earlier commands fail\n    - DO NOT use newlines to separate commands (newlines are ok in quoted strings)\n  - Try to maintain your current working directory throughout the session by using absolute paths and avoiding usage of `cd`. You may use `cd` if the User explicitly requests it.\n    <good-example>\n    pytest /foo/bar/tests\n    </good-example>\n    <bad-example>\n    cd /foo/bar && pytest tests\n    </bad-example>",
	}
)

type BashInput struct {
	Command         string `json:"command" jsonschema:"The command to execute"`
	Description     string `json:"description,omitempty" jsonschema:"Clear, concise description of what this command does in 5-10 words, in active voice. Examples:\nInput: ls\nOutput: List files in current directory\n\nInput: git status\nOutput: Show working tree status\n\nInput: npm install\nOutput: Install package dependencies\n\nInput: mkdir foo\nOutput: Create directory 'foo'"`
	RunInBackground bool   `json:"run_in_background,omitempty" jsonschema:"Set to true to run this command in the background. Use BashOutput to read the output later."`
	Timeout         int64  `json:"timeout,omitempty" jsonschema:"Optional timeout in milliseconds (max 600000)"`
}

type BashResult struct {
	Result string `json:"result"`
}

func Bash(ctx context.Context, req *sdk.CallToolRequest, args BashInput) (*sdk.CallToolResult, any, error) {
	server := GetState()
	result, err := server.executeBashCommand(ctx, args.Command, args.Description, args.Timeout, args.RunInBackground)
	if err != nil {
		return nil, nil, err
	}

	output := &BashResult{Result: result}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: result}},
		StructuredContent: output,
	}, output, nil
}
