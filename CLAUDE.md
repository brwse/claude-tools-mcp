# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is an MCP (Model Context Protocol) server that exposes Claude Code's file and shell tools over HTTP, allowing other MCP clients to use these tools remotely. The server runs in stateless mode for horizontal scaling.

## Development Commands

### Building
```bash
go build -o claude-tools-mcp ./cmd/claude-tools-mcp
```

### Running Tests
```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/tools

# Run a specific test
go test -run TestFunctionName ./internal/tools
```

### Running the Server
```bash
# Default (localhost:8080)
go run ./cmd/claude-tools-mcp

# Custom address
go run ./cmd/claude-tools-mcp --addr localhost:9000
```

## Architecture

### Core Components

**cmd/claude-tools-mcp/main.go**: HTTP server entrypoint
- Uses Cobra for CLI
- Implements graceful shutdown with SIGINT/SIGTERM handling
- Configures security timeouts (ReadHeaderTimeout, IdleTimeout) to prevent slowloris attacks
- Runs MCP server in stateless mode via `mcp.NewStreamableHTTPHandler`

**internal/tools/server.go**: Global state management
- `State` struct manages file access tracking and background shells
- Synchronized via `sync.RWMutex` for concurrent access
- `ReadFiles` map tracks file modification times to detect external changes
- `BackgroundShells` map manages long-running bash processes
- Singleton instance via `GetState()`

**Tool Implementations** (internal/tools/):
- Each tool has its own file (bash.go, read.go, write.go, edit.go, glob.go, grep.go)
- Tools follow MCP SDK patterns: define `Tool` schema and handler function
- Handler signature: `func(ctx context.Context, req *sdk.CallToolRequest, args InputType) (*sdk.CallToolResult, any, error)`

### Key Design Patterns

**File Safety**:
- `resolvePath()` rejects relative paths to prevent directory traversal
- Edit tool requires files to be read first (tracked in `State.ReadFiles`)
- Edit tool detects external modifications by comparing ModTime against last read
- All file paths must be absolute

**Output Size Constraints** (internal/tools/constraints.go):
- Files: 10MB max (prevents memory exhaustion)
- Tool output: ~100k tokens max (~400k chars, estimated at 4 chars/token)
- Grep/glob results: 1000 lines max
- Read tool: 2000 lines default, truncates lines >2000 chars

**Background Shell Management**:
- `BackgroundShell` struct tracks running processes with `Done` channel
- `SyncBuffer` wraps `bytes.Buffer` with mutex for concurrent read/write
- Output tracked with `LastStdoutReadAt`/`LastStderrReadAt` byte positions
- Shell IDs generated sequentially: "shell_1", "shell_2", etc.

**Line Number Formatting**:
- `catN()` formats output like `cat -n` with dynamic column width
- Format: `%6d→content` (minimum 6 chars for line numbers, arrow separator)
- Used by read tool and edit tool context display

**Edit Tool Context Display**:
- `modifiedLines()` finds first/last differing lines between old/new content
- Expands by `delta` lines (default 2) for context
- Single edits show context snippet; `replace_all` edits show summary only

**Grep Tool (Ripgrep Wrapper)**:
- Three output modes: `files_with_matches` (default), `content`, `count`
- Context flags (-A, -B, -C) and line numbers (-n) only apply in `content` mode
- Multiline mode requires both `--multiline` and `--multiline-dotall` flags
- Exit code 1 (no matches) treated as success with empty output

## Testing Notes

- Tests use `testify` for assertions
- Each tool has its own `*_test.go` file
- Tests create temporary files/directories for isolation
- State is reset between tests via `NewState()`
