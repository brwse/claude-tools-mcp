# Claude Tools MCP Server

An MCP (Model Context Protocol) server that exposes Claude Code's file and shell manipulation tools over HTTP, allowing any MCP client to use these tools remotely.

## Features

This server provides the following tools:

- **bash**: Execute shell commands with timeout support and background execution
- **bash_output**: Retrieve output from background shell processes
- **kill_shell**: Terminate background shell processes
- **read**: Read files with line offset/limit support
- **write**: Write files to disk
- **edit**: Perform exact string replacements in files
- **glob**: Find files using glob patterns
- **grep**: Search file contents using ripgrep (regex support, multiple output modes)

## Installation

### From Source

```bash
go build -o claude-tools-mcp ./cmd/claude-tools-mcp
```

### With Docker

```bash
docker build -t claude-tools-mcp .
docker run -p 8080:8080 claude-tools-mcp
```

### From Smithery

This server is available on [Smithery](https://smithery.ai/), the MCP server registry:

```bash
npx @smithery/cli install claude-tools-mcp
```

## Usage

### Starting the Server

```bash
# Default (localhost:8080)
./claude-tools-mcp

# Custom address
./claude-tools-mcp --addr localhost:9000
```

### With Docker

```bash
# Default port (8080)
docker run -p 8080:8080 claude-tools-mcp

# Custom port
docker run -e PORT=9000 -p 9000:9000 claude-tools-mcp
```

### Configuration

The server runs in stateless mode, allowing each HTTP request to be handled independently. This enables horizontal scaling and simpler deployment.

### Security Features

- **Timeout protection**: Prevents slowloris attacks with ReadHeaderTimeout and IdleTimeout
- **Graceful shutdown**: Responds to SIGINT/SIGTERM, allowing in-flight requests to complete
- **Path validation**: Rejects relative paths to prevent directory traversal
- **File size limits**: 10MB max file size, ~100k token max output
- **Result limits**: Maximum 1000 lines for grep/glob results

## Architecture

The server uses the [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) to expose tools over HTTP. All tools are stateless except for:

- **File modification tracking**: Detects when files are edited externally
- **Background shell management**: Tracks long-running bash processes

See [CLAUDE.md](./CLAUDE.md) for detailed architecture documentation.

## Development

### Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./internal/tools

# Specific test
go test -run TestFunctionName ./internal/tools
```

### Dependencies

- Go 1.25.1+
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- [Cobra](https://github.com/spf13/cobra) for CLI
- [mimetype](https://github.com/gabriel-vasile/mimetype) for file type detection
- ripgrep (`rg`) must be installed for the grep tool

## License

Part of the [brwse](https://github.com/brwse/brwse) third-party tools collection.
