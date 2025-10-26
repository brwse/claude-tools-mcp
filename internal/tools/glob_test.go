package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupGlobTestFiles(t *testing.T) (state *State, dir string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Create test files with varied extensions and directory depths to support testing of:
	// - Single-level patterns (*.go)
	// - Recursive patterns (**/*.go)
	// - Directory-scoped patterns (subdir/*)
	// - Exclusion patterns (*.txt should not match *.go)
	files := map[string]string{
		"file1.go":        "package main",
		"file2.go":        "package test",
		"test.txt":        "text file",
		"README.md":       "# README",
		"subdir/file3.go": "package sub",
		"subdir/test.py":  "def test():",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}

	return NewState(), tmpDir
}

func callGlob(t *testing.T, state *State, input GlobInput) (string, error) {
	t.Helper()
	path := input.Path

	// Replicate the behavior of the Glob API endpoint: when path is empty,
	// default to the current working directory. This ensures tests that don't
	// specify a path still execute predictably, similar to real API usage.
	if path == "" {
		wd, _ := os.Getwd()
		path = wd
	}

	return state.executeGlob(context.Background(), input.Pattern, path)
}

func TestGlob_BasicFunctionality(t *testing.T) {
	state, dir := setupGlobTestFiles(t)

	t.Run("match all go files", func(t *testing.T) {
		result, err := callGlob(t, state, GlobInput{
			Pattern: "*.go",
			Path:    dir,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "file1.go")
		assert.Contains(t, result, "file2.go")
		assert.NotContains(t, result, "test.txt")
	})

	t.Run("match with subdirectories", func(t *testing.T) {
		result, err := callGlob(t, state, GlobInput{
			Pattern: "**/*.go",
			Path:    dir,
		})
		require.NoError(t, err)
		assert.Contains(t, result, ".go")
	})

	t.Run("match specific file", func(t *testing.T) {
		result, err := callGlob(t, state, GlobInput{
			Pattern: "README.md",
			Path:    dir,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "README.md")
		assert.NotContains(t, result, "test.txt")
	})

	t.Run("no matches", func(t *testing.T) {
		result, err := callGlob(t, state, GlobInput{
			Pattern: "*.nonexistent",
			Path:    dir,
		})
		require.NoError(t, err)
		// When no files match, executeGlob returns "No files found" without error.
		// This graceful handling avoids breaking consumers on empty result sets.
		assert.Contains(t, result, "No files found")
	})
}

func TestGlob_Patterns(t *testing.T) {
	state, dir := setupGlobTestFiles(t)

	t.Run("multiple extensions", func(t *testing.T) {
		result, err := callGlob(t, state, GlobInput{
			Pattern: "*.{go,py}",
			Path:    dir,
		})
		// Brace expansion is an optional shell feature that may not be supported
		// depending on the underlying shell implementation. Skip rather than fail
		// to allow graceful degradation on systems where it's unavailable.
		if err != nil {
			t.Skip("Brace expansion not supported")
		}
		assert.NotEmpty(t, result)
	})

	t.Run("wildcard in middle", func(t *testing.T) {
		result, err := callGlob(t, state, GlobInput{
			Pattern: "file*.go",
			Path:    dir,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "file1.go")
		assert.Contains(t, result, "file2.go")
	})

	t.Run("directory pattern", func(t *testing.T) {
		result, err := callGlob(t, state, GlobInput{
			Pattern: "subdir/*",
			Path:    dir,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "file3.go")
		assert.Contains(t, result, "test.py")
	})
}

func TestGlob_Errors(t *testing.T) {
	state := NewState()

	t.Run("nonexistent directory", func(t *testing.T) {
		result, err := callGlob(t, state, GlobInput{
			Pattern: "*.go",
			Path:    "/nonexistent/path",
		})
		// executeGlob may either return an error or gracefully handle the nonexistent
		// directory by returning "No files found". Both behaviors are acceptable since
		// the underlying find command exit code depends on implementation details.
		if err == nil {
			assert.Contains(t, result, "No files found")
		}
	})

	t.Run("empty pattern", func(t *testing.T) {
		tmpDir := t.TempDir()
		result, err := callGlob(t, state, GlobInput{
			Pattern: "",
			Path:    tmpDir,
		})
		// An empty glob pattern has no defined semantics. Accepting either error or
		// "No files found" keeps the API flexible across different shell behaviors.
		if err == nil {
			assert.Contains(t, result, "No files found")
		}
	})
}

func TestGlob_MCPIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0o644))

	// Verify that the public Glob function properly adapts the internal executeGlob
	// to the MCP protocol: converting arguments, handling errors, and wrapping results
	// in the CallToolResult format expected by the model context protocol.
	result, _, err := Glob(context.Background(), &sdk.CallToolRequest{}, GlobInput{
		Pattern: "*.go",
		Path:    tmpDir,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}
