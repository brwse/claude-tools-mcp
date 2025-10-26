package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *State) executeWrite(ctx context.Context, filePath, content string) (string, error) {
	resolved, err := resolvePath(filePath)
	if err != nil {
		return "", err
	}

	// For existing files, enforce a read-before-write constraint to prevent accidental overwrites
	// of files the user hasn't explicitly read first. This safeguard requires that either:
	// (1) the file was previously read in this session, or (2) the file is being created new.
	// Additionally, detect if the file has been modified externally since it was last read,
	// which would indicate stale state and require a fresh read before proceeding.
	if fileInfo, err := os.Stat(resolved); err == nil {
		s.Mu.RLock()
		readTime, wasRead := s.ReadFiles[resolved]
		s.Mu.RUnlock()

		if !wasRead {
			return "", fmt.Errorf("file exists, you must read it first before writing")
		}

		if fileInfo.ModTime().After(readTime) {
			return "", fmt.Errorf("file has been modified since last read, please read again before writing")
		}
	}

	// Create parent directories if they don't exist to support writing to nested paths
	_ = os.MkdirAll(filepath.Dir(resolved), 0o750)

	err = os.WriteFile(resolved, []byte(content), 0o600)
	if err != nil {
		return "", fmt.Errorf("Cannot write file: %s", err)
	}

	// Determine whether this is a new file or an update to generate appropriate user feedback
	message := "File created successfully at: " + resolved
	s.Mu.RLock()
	_, wasRead := s.ReadFiles[resolved]
	s.Mu.RUnlock()
	if wasRead {
		message = "File updated successfully at: " + resolved
	}

	// Update the cached modification time for this file to establish the current state.
	// This enables future write operations to detect external changes via timestamp comparison.
	s.Mu.Lock()
	if fileInfo, err := os.Stat(resolved); err == nil {
		s.ReadFiles[resolved] = fileInfo.ModTime()
	}
	s.Mu.Unlock()

	return message, nil
}

var WriteTool = sdk.Tool{
	Name:        "write",
	Description: "Writes a file to the local filesystem.\n\nUsage:\n- This tool will overwrite the existing file if there is one at the provided path.\n- If this is an existing file, you MUST use the Read tool first to read the file's contents. This tool will fail if you did not read the file first.\n- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.\n- NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.\n- Only use emojis if the user explicitly requests it. Avoid writing emojis to files unless asked.",
}

type WriteInput struct {
	FilePath string `json:"file_path" jsonschema:"The absolute path to the file to write (must be absolute, not relative)"`
	Content  string `json:"content" jsonschema:"The content to write to the file"`
}
type WriteOutput struct {
	Message string `json:"message"`
}

func Write(ctx context.Context, req *sdk.CallToolRequest, args WriteInput) (*sdk.CallToolResult, any, error) {
	server := GetState()
	result, err := server.executeWrite(ctx, args.FilePath, args.Content)
	if err != nil {
		return nil, nil, err
	}
	output := &WriteOutput{Message: result}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: result}},
		StructuredContent: output,
	}, output, nil
}
