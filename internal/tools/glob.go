package tools

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *State) executeGlob(ctx context.Context, pattern, path string) (string, error) {
	// Reject patterns containing null bytes to prevent shell injection via null-byte splitting attacks.
	if strings.Contains(pattern, "\x00") {
		return "", fmt.Errorf("Invalid glob pattern.")
	}

	searchDir := "."
	if path != "" {
		resolved, err := resolvePath(path)
		if err != nil {
			return "", err
		}
		searchDir = resolved
	}

	findPattern := filepath.Join(searchDir, pattern)

	// Use find + xargs with -print0/-0 delimiters to safely handle filenames with spaces and special chars.
	// ls -t sorts results by modification time (most recent first) per the documented behavior.
	// xargs -r prevents running ls when find produces no output, avoiding spurious cwd listings.
	// Redirect stderr to suppress errors for unmatched patterns, returning "No files found" instead.
	cmd := exec.CommandContext(ctx, "sh", "-c",
		"find "+shellescape(searchDir)+" -type f -path "+shellescape(findPattern)+" -print0 | xargs -0 -r ls -t 2>/dev/null")
	output, err := cmd.Output()
	if err != nil {
		return "No files found", nil
	}

	if len(output) == 0 {
		return "No files found", nil
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return "No files found", nil
	}

	result = limitLines(ctx, result)
	if err := checkOutputSize(ctx, result, "glob"); err != nil {
		return "", err
	}

	return result, nil
}

func shellescape(s string) string {
	// Escape string for safe shell execution using single-quote wrapping.
	// Replaces ' with '\"'\"' to break out of single quotes, insert a literal quote via double quotes, and re-enter single quotes.
	// This prevents shell metacharacter injection even if the string contains special chars like $, `, \, etc.
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

var GlobTool = sdk.Tool{
	Name:        "glob",
	Description: "- Fast file pattern matching tool that works with any codebase size\n- Supports glob patterns like \"**/*.js\" or \"src/**/*.ts\"\n- Returns matching file paths sorted by modification time\n- Use this tool when you need to find files by name patterns\n- When you are doing an open ended search that may require multiple rounds of globbing and grepping, use the Agent tool instead\n- You can call multiple tools in a single response. It is always better to speculatively perform multiple searches in parallel if they are potentially useful.",
}

type GlobInput struct {
	Pattern string `json:"pattern" jsonschema:"The glob pattern to match files against"`
	Path    string `json:"path,omitempty" jsonschema:"The directory to search in. If not specified, the working directory will be used"`
}
type GlobOutput struct {
	Files string `json:"files"`
}

func Glob(ctx context.Context, req *sdk.CallToolRequest, args GlobInput) (*sdk.CallToolResult, any, error) {
	server := GetState()
	result, err := server.executeGlob(ctx, args.Pattern, args.Path)
	if err != nil {
		return nil, nil, err
	}
	output := &GlobOutput{Files: result}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: result}},
		StructuredContent: output,
	}, output, nil
}
