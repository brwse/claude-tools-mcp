package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func (s *State) executeGrep(ctx context.Context, pattern, path, outputMode, glob, typeFilter string,
	caseInsensitive, multiline, lineNumber bool, contextAfter, contextBefore, contextAround, headLimit int,
) (string, error) {
	rgArgs, err := buildRipgrepArgs(outputMode, glob, typeFilter, caseInsensitive, multiline, lineNumber,
		int64(contextAfter), int64(contextBefore), int64(contextAround))
	if err != nil {
		return "", err
	}

	// Pattern must come after "--" to prevent it from being interpreted as a flag by ripgrep
	rgArgs = append(rgArgs, "--", pattern)
	if path != "" {
		searchPath, err := resolvePath(path)
		if err != nil {
			return "", err
		}
		rgArgs = append(rgArgs, searchPath)
	}

	output, err := execRipgrep(ctx, rgArgs...)
	if err != nil {
		return "", err
	}

	// Apply user-requested headLimit first, then system-wide constraints (limitLines, checkOutputSize)
	output = applyHeadLimit(output, int(headLimit))
	output = strings.TrimSpace(output)
	if output == "" {
		return "No matches found", nil
	}

	// limitLines enforces absolute max result count; checkOutputSize enforces max token output
	output = limitLines(ctx, output)
	if err := checkOutputSize(ctx, output, "grep"); err != nil {
		return "", err
	}

	return output, nil
}

func buildRipgrepArgs(outputMode, glob, typeFilter string, caseInsensitive, multiline, lineNumber bool,
	contextAfter, contextBefore, contextAround int64,
) ([]string, error) {
	rgArgs := []string{}
	if outputMode == "" {
		// Default to files_with_matches when user doesn't specify output mode
		outputMode = "files_with_matches"
	}

	// Map high-level output modes to ripgrep CLI flags
	switch outputMode {
	case "files_with_matches":
		rgArgs = append(rgArgs, "--files-with-matches")
	case "count":
		rgArgs = append(rgArgs, "--count")
	case "content":
		// Context flags only apply in content mode; they're ignored by ripgrep in other modes
		if contextAfter > 0 {
			rgArgs = append(rgArgs, fmt.Sprintf("-A%d", contextAfter))
		}
		if contextBefore > 0 {
			rgArgs = append(rgArgs, fmt.Sprintf("-B%d", contextBefore))
		}
		if contextAround > 0 {
			rgArgs = append(rgArgs, fmt.Sprintf("-C%d", contextAround))
		}
		if lineNumber {
			rgArgs = append(rgArgs, "--line-number")
		}
	default:
		return nil, fmt.Errorf("Invalid output_mode: %s. Must be one of: content, files_with_matches, count.", outputMode)
	}

	// Apply global filter options
	if caseInsensitive {
		rgArgs = append(rgArgs, "--ignore-case")
	}

	// Multiline matching requires both flags: --multiline enables cross-line patterns,
	// --multiline-dotall makes . match newlines
	if multiline {
		rgArgs = append(rgArgs, "--multiline", "--multiline-dotall")
	}

	if typeFilter != "" {
		rgArgs = append(rgArgs, "--type", typeFilter)
	}
	if glob != "" {
		rgArgs = append(rgArgs, "--glob", glob)
	}

	return rgArgs, nil
}

func execRipgrep(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "rg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Ripgrep exit codes: 1 = no matches found (normal), 2 = no files searched (error),
			// other codes = actual failures
			if exitErr.ExitCode() == 1 {
				// No matches is not an error; return empty string with nil error
				return "", nil
			}
			if exitErr.ExitCode() == 2 {
				return "", fmt.Errorf("No files were searched. This usually means ripgrep applied a filter that excluded all files.")
			}
			return "", fmt.Errorf("rg exited with code %d:\n%s", exitErr.ExitCode(), output)
		}
		return "", fmt.Errorf("Failed to execute rg: %s", err)
	}
	return string(output), nil
}

func applyHeadLimit(output string, limit int) string {
	if limit <= 0 {
		return output
	}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > limit {
		lines = lines[:limit]
	}
	return strings.Join(lines, "\n")
}

var GrepTool = sdk.Tool{
	Name:        "grep",
	Description: "A powerful search tool built on ripgrep\n\n  Usage:\n  - ALWAYS use Grep for search tasks. NEVER invoke `grep` or `rg` as a Bash command. The Grep tool has been optimized for correct permissions and access.\n  - Supports full regex syntax (e.g., \"log.*Error\", \"function\\\\s+\\\\w+\")\n  - Filter files with glob parameter (e.g., \"*.js\", \"**/*.tsx\") or type parameter (e.g., \"js\", \"py\", \"rust\")\n  - Output modes: \"content\" shows matching lines, \"files_with_matches\" shows only file paths (default), \"count\" shows match counts\n  - Use Task tool for open-ended searches requiring multiple rounds\n  - Pattern syntax: Uses ripgrep (not grep) - literal braces need escaping (use `interface\\\\{\\\\}` to find `interface{}` in Go code)\n  - Multiline matching: By default patterns match within single lines only. For cross-line patterns like `struct \\\\{[\\\\s\\\\S]*?field`, use `multiline: true`\n",
}

// GrepInput represents parameters for the grep/ripgrep search.
// JSON tag names for A, B, C, N, I follow ripgrep CLI conventions (-A, -B, -C, -n, -i)
// to provide familiar naming to users familiar with ripgrep/grep command-line tools.
type GrepInput struct {
	Pattern    string `json:"pattern" jsonschema:"The regular expression pattern to search for in file contents"`
	Path       string `json:"path,omitempty" jsonschema:"File or directory to search in. Defaults to working directory"`
	Glob       string `json:"glob,omitempty" jsonschema:"Glob pattern to filter files (e.g. *.go)"`
	Type       string `json:"type,omitempty" jsonschema:"File type to search (e.g. go, py). More efficient than include for standard file types"`
	OutputMode string `json:"output_mode,omitempty" jsonschema:"Output mode: 'content' shows matching lines, 'files_with_matches' shows file paths (default), 'count' shows match counts"`
	A          int    `json:"-A,omitempty" jsonschema:"Number of lines to show after each match. Requires output_mode: content"`
	B          int    `json:"-B,omitempty" jsonschema:"Number of lines to show before each match. Requires output_mode: content"`
	C          int    `json:"-C,omitempty" jsonschema:"Number of lines to show before and after each match. Requires output_mode: content"`
	N          bool   `json:"-n,omitempty" jsonschema:"Show line numbers in output. Requires output_mode: content"`
	I          bool   `json:"-i,omitempty" jsonschema:"Case insensitive search"`
	Multiline  bool   `json:"multiline,omitempty" jsonschema:"Enable multiline mode where patterns can span lines. Default: false"`
	HeadLimit  int    `json:"head_limit,omitempty" jsonschema:"Limit output to first N lines/entries"`
}
type GrepOutput struct {
	Results string `json:"results"`
}

func Grep(ctx context.Context, req *sdk.CallToolRequest, args GrepInput) (*sdk.CallToolResult, any, error) {
	server := GetState()
	result, err := server.executeGrep(ctx, args.Pattern, args.Path, args.OutputMode, args.Glob, args.Type,
		args.I, args.Multiline, args.N,
		args.A, args.B, args.C, args.HeadLimit)
	if err != nil {
		return nil, nil, err
	}
	output := &GrepOutput{Results: result}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: result}},
		StructuredContent: output,
	}, output, nil
}
