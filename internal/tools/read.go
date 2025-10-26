package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *State) executeRead(ctx context.Context, filePath string, offset, limit int64) (string, error) {
	resolved, err := resolvePath(filePath)
	if err != nil {
		return "", err
	}

	fileInfo, err := validateFileForRead(ctx, resolved)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("Cannot read file: %s", err)
	}

	// Track modification time for files that have been read, enabling change detection
	// for features that may depend on knowing when a file was last accessed
	s.Mu.Lock()
	s.ReadFiles[resolved] = fileInfo.ModTime()
	s.Mu.Unlock()

	if len(content) == 0 {
		return "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>", nil
	}

	mtype, err := mimetype.DetectFile(resolved)
	if err != nil {
		return "", fmt.Errorf("Cannot detect file type: %s", err)
	}

	// Reject binary files like images and audio; only display text-like content
	switch strings.Split(mtype.String(), "/")[0] {
	case "image", "audio":
		return fmt.Sprintf("[Binary file: %s (%s), %d bytes]", resolved, mtype.String(), len(content)), nil
	default:
		if !mtype.Is("text/plain") && !mtype.Parent().Is("text/plain") {
			return fmt.Sprintf("[Binary file: %s (%s), %d bytes]", resolved, mtype.String(), len(content)), nil
		}
	}

	lines := strings.Split(string(content), "\n")
	totalLines := len(lines)
	startLine, endLine := calculateLineRange(totalLines, int(offset), int(limit))

	// When user provides an offset, validate it points to a valid line in the file
	if offset > 0 && (startLine < 1 || startLine > totalLines) {
		return fmt.Sprintf(
			"<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>",
			startLine,
			totalLines,
		), nil
	}

	selectedLines := lines[startLine-1 : endLine]
	result := catN(selectedLines, startLine)

	if err := checkOutputSize(ctx, result, "read"); err != nil {
		return "", err
	}

	return result, nil
}

func validateFileForRead(ctx context.Context, resolved string) (os.FileInfo, error) {
	fileInfo, err := os.Stat(resolved)
	if os.IsNotExist(err) || (err == nil && fileInfo.IsDir()) {
		return nil, fmt.Errorf("file does not exist")
	}
	if err := checkFileSize(ctx, fileInfo.Size(), "read"); err != nil {
		return nil, err
	}
	return fileInfo, nil
}

func calculateLineRange(totalLines, offset, limit int) (start, end int) {
	start = 1
	if offset > 0 {
		start = offset
	}

	end = totalLines
	if limit > 0 {
		end = min(start+limit-1, totalLines)
	}

	// Default behavior: cap at 2000 lines when neither offset nor limit are provided
	// This prevents expensive operations on very large files while still allowing
	// explicit control via offset/limit parameters
	if limit == 0 && offset == 0 && totalLines > 2000 {
		end = 2000
	}

	return start, end
}

var ReadTool = sdk.Tool{
	Name:        "read",
	Description: "Reads a file from the local filesystem. You can access any file directly by using this tool.\nAssume this tool is able to read all files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error will be returned.\n\nUsage:\n- The file_path parameter must be an absolute path, not a relative path\n- By default, it reads up to 2000 lines starting from the beginning of the file\n- You can optionally specify a line offset and limit (especially handy for large files), but it's recommended to read the whole file by not providing these parameters\n- Any lines longer than 2000 characters will be truncated\n- Results are returned using cat -n format, with line numbers starting at 1\n- This tool can only read files, not directories. To read a directory, use an ls command via the Bash tool.\n- You can call multiple tools in a single response. It is always better to speculatively read multiple potentially useful files in parallel.\n- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents.",
}

type ReadInput struct {
	FilePath string `json:"file_path" jsonschema:"The absolute path to the file to read"`
	Offset   int64  `json:"offset,omitempty" jsonschema:"The line number to start reading from. Only provide if the file is too large to read at once"`
	Limit    int64  `json:"limit,omitempty" jsonschema:"The number of lines to read. Only provide if the file is too large to read at once"`
}
type ReadOutput struct {
	Content string `json:"content"`
}

func Read(ctx context.Context, req *sdk.CallToolRequest, args ReadInput) (*sdk.CallToolResult, any, error) {
	server := GetState()
	result, err := server.executeRead(ctx, args.FilePath, args.Offset, args.Limit)
	if err != nil {
		return nil, nil, err
	}
	output := &ReadOutput{Content: result}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: result}},
		StructuredContent: output,
	}, output, nil
}
