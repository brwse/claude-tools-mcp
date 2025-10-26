package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGrepTestFiles(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	files := map[string]string{
		"file1.go":  "package main\nfunc main() {\n\tfmt.Println(\"Hello\")\n}",
		"file2.go":  "package test\nfunc TestSomething() {}",
		"file3.txt": "This is a text file\nwith multiple lines\nand the word pattern appears here",
		"README.md": "# Documentation\nThis is documentation",
	}
	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}
	return tmpDir
}

func TestGrep_BasicFunctionality(t *testing.T) {
	dir := setupGrepTestFiles(t)

	t.Run("find pattern in files", func(t *testing.T) {
		result, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern: "pattern",
			Path:    dir,
		})
		require.NoError(t, err)
		textContent := result.Content[0].(*sdk.TextContent)
		assert.Contains(t, textContent.Text, "file3.txt")
	})

	t.Run("content output mode", func(t *testing.T) {
		// Verify "content" mode returns matching lines themselves, not just file paths
		result, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern:    "Hello",
			Path:       dir,
			OutputMode: "content",
		})
		require.NoError(t, err)
		textContent := result.Content[0].(*sdk.TextContent)
		assert.Contains(t, textContent.Text, "Hello")
	})

	t.Run("case insensitive search", func(t *testing.T) {
		// Case insensitive flag enables uppercase pattern to match lowercase text
		result, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern:    "HELLO",
			Path:       dir,
			OutputMode: "content",
			I:          true,
		})
		require.NoError(t, err)
		textContent := result.Content[0].(*sdk.TextContent)
		assert.Contains(t, textContent.Text, "Hello")
	})

	t.Run("no matches", func(t *testing.T) {
		// When ripgrep finds no matches (exit code 1), executeGrep converts this to "No matches found"
		// rather than returning an error, providing a graceful user-facing response
		result, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern: "nonexistent",
			Path:    dir,
		})
		require.NoError(t, err)
		textContent := result.Content[0].(*sdk.TextContent)
		assert.Contains(t, textContent.Text, "No matches found")
	})
}

func TestGrep_Filters(t *testing.T) {
	dir := setupGrepTestFiles(t)

	t.Run("glob filter", func(t *testing.T) {
		// Glob filter restricts search to files matching the pattern, excluding non-matching files
		result, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern: "func",
			Path:    dir,
			Glob:    "*.go",
		})
		require.NoError(t, err)
		textContent := result.Content[0].(*sdk.TextContent)
		assert.Contains(t, textContent.Text, "file1.go")
		assert.Contains(t, textContent.Text, "file2.go")
		assert.NotContains(t, textContent.Text, "file3.txt")
	})

	t.Run("type filter", func(t *testing.T) {
		// Type filter uses ripgrep's built-in file type definitions for more reliable filtering
		// than glob patterns, automatically including file extension patterns for the type
		result, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern: "package",
			Path:    dir,
			Type:    "go",
		})
		require.NoError(t, err)
		textContent := result.Content[0].(*sdk.TextContent)
		assert.Contains(t, textContent.Text, "file1.go")
		assert.Contains(t, textContent.Text, "file2.go")
	})
}

func TestGrep_Patterns(t *testing.T) {
	dir := setupGrepTestFiles(t)

	t.Run("regex pattern", func(t *testing.T) {
		// Verify ripgrep's regex engine supports standard regex syntax (\s, \w, etc.)
		result, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern:    `func\s+\w+`,
			Path:       dir,
			OutputMode: "content",
		})
		require.NoError(t, err)
		textContent := result.Content[0].(*sdk.TextContent)
		assert.Contains(t, textContent.Text, "func")
	})

	t.Run("multiline pattern", func(t *testing.T) {
		// Multiline flag enables cross-line matching; ripgrep requires both --multiline and
		// --multiline-dotall to allow patterns like "package.*func" to match across newlines
		_, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern:    `package.*func`,
			Path:       dir,
			OutputMode: "content",
			Multiline:  true,
		})
		require.NoError(t, err)
	})
}

func TestGrep_OutputModes(t *testing.T) {
	dir := setupGrepTestFiles(t)

	t.Run("files_with_matches mode", func(t *testing.T) {
		// files_with_matches (default) returns only file paths containing matches, one per line
		result, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern:    "package",
			Path:       dir,
			OutputMode: "files_with_matches",
		})
		require.NoError(t, err)
		textContent := result.Content[0].(*sdk.TextContent)
		assert.Contains(t, textContent.Text, "file")
		lines := strings.Split(strings.TrimSpace(textContent.Text), "\n")
		assert.Greater(t, len(lines), 0)
	})

	t.Run("count mode", func(t *testing.T) {
		// count mode returns the number of matches per file, providing aggregate statistics
		result, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern:    "func",
			Path:       dir,
			OutputMode: "count",
		})
		require.NoError(t, err)
		textContent := result.Content[0].(*sdk.TextContent)
		assert.NotEmpty(t, textContent.Text)
	})
}

func TestGrep_Errors(t *testing.T) {
	t.Run("nonexistent path", func(t *testing.T) {
		// ripgrep (and thus execRipgrep) returns a non-zero exit code when the search path doesn't exist
		_, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern: "pattern",
			Path:    "/nonexistent/path",
		})
		require.Error(t, err)
	})

	t.Run("invalid output mode", func(t *testing.T) {
		// buildRipgrepArgs validates output mode against known modes and rejects invalid values
		tmpDir := t.TempDir()
		_, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern:    "pattern",
			Path:       tmpDir,
			OutputMode: "invalid",
		})
		require.Error(t, err)
	})

	t.Run("relative path rejected", func(t *testing.T) {
		// resolvePath enforces absolute paths only for security; relative paths could access
		// unintended directories depending on where ripgrep is invoked from
		_, _, err := Grep(context.Background(), &sdk.CallToolRequest{}, GrepInput{
			Pattern: "pattern",
			Path:    "./relative",
		})
		require.Error(t, err)
	})
}
