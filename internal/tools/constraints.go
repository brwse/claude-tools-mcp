package tools

import (
	"context"
	"fmt"
)

const (
	// 10MB hard limit prevents reading extremely large files that would cause excessive
	// memory usage and API token consumption when returned to the client.
	absoluteMaxFileSize = 10 * 1024 * 1024

	// ~100k tokens expressed as character count (Claude tokenizes at roughly 4 chars per token).
	// This prevents responses from consuming excessive tokens and hitting API limits.
	absoluteMaxOutputSize = 25_000 * 4

	// Maximum lines to return from grep/glob results. This truncates outputs from commands
	// that match thousands of files, preventing massive responses and prompt bloat.
	absoluteMaxResults = 1000
)

func checkFileSize(ctx context.Context, size int64, toolName string) error {
	effectiveMax := int64(absoluteMaxFileSize)
	if size > effectiveMax {
		return fmt.Errorf(
			"File content (%d bytes) exceeds maximum allowed size (%d bytes). Please use offset and limit parameters to read specific portions of the file, or use the Grep tool to search for specific content.",
			size,
			effectiveMax,
		)
	}
	return nil
}

// checkOutputSize validates that tool output doesn't exceed token limits before returning to client.
// Provides tool-specific guidance to help users reduce output when it exceeds limits (e.g., "use grep
// to search" for read tool, "use head_limit" for grep tool). Token count is estimated using the
// common approximation of 4 characters per token.
func checkOutputSize(ctx context.Context, output, toolName string) error {
	effectiveMax := absoluteMaxOutputSize
	if len(output) > effectiveMax {
		var suggestion string
		switch toolName {
		case "read":
			suggestion = "Use the offset and limit parameters to read specific portions of the file, or use the Grep tool to search for specific content."
		case "write":
			suggestion = "Consider breaking the file into smaller chunks."
		case "edit":
			suggestion = "Consider editing smaller sections of the file."
		case "grep":
			suggestion = "Consider using the head_limit parameter to restrict results, adding more specific patterns, or using glob/type filters to narrow the search."
		case "glob":
			suggestion = "Consider using more specific glob patterns to narrow the search scope."
		case "bash":
			suggestion = "Consider using background execution with BashOutput to stream results, or redirect output to a file and read specific portions."
		default:
			suggestion = "Consider breaking down the operation into smaller parts or using more specific parameters to limit output."
		}
		return fmt.Errorf(
			"Output (%d tokens) exceeds maximum allowed size (%d tokens). %s",
			len(output)/4,
			effectiveMax/4,
			suggestion,
		)
	}
	return nil
}

// limitLines truncates output to at most absoluteMaxResults lines. Used by grep and glob to prevent
// catastrophic output when patterns match thousands of results. Returns the substring up to and
// including the Nth newline character (not just a count) to preserve complete lines.
func limitLines(ctx context.Context, s string) string {
	if s == "" {
		return s
	}
	effectiveMax := absoluteMaxResults
	count := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			count++
			if count >= effectiveMax {
				// Return substring including the Nth newline to keep the final line complete.
				return s[:i+1]
			}
		}
	}
	return s
}
