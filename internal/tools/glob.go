package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// fileInfo holds file path and modification time for sorting
type fileInfo struct {
	path    string
	modTime time.Time
}

func (s *State) executeGlob(ctx context.Context, pattern, path string) (string, error) {
	// Reject patterns containing null bytes to prevent potential security issues
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

	// Check if searchDir exists and is accessible
	if _, err := os.Stat(searchDir); err != nil {
		return "No files found", nil
	}

	var matches []fileInfo

	// Use doublestar for proper glob matching with ** support
	fsys := os.DirFS(searchDir)
	err := doublestar.GlobWalk(fsys, pattern, func(path string, d fs.DirEntry) error {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Only match files, not directories
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			// Skip files we can't stat
			return nil
		}

		matches = append(matches, fileInfo{
			path:    path,
			modTime: info.ModTime(),
		})

		return nil
	})

	if err != nil && err != context.Canceled {
		return "", err
	}

	if len(matches) == 0 {
		return "No files found", nil
	}

	// Sort by modification time (most recent first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime.After(matches[j].modTime)
	})

	// Build result string
	var result strings.Builder
	for i, match := range matches {
		if i > 0 {
			result.WriteByte('\n')
		}
		result.WriteString(match.path)
	}

	resultStr := result.String()
	resultStr = limitLines(ctx, resultStr)
	if err := checkOutputSize(ctx, resultStr, "glob"); err != nil {
		return "", err
	}

	return resultStr, nil
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
