package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFileForEdit(t *testing.T, content string) (state *State, path string) {
	t.Helper()
	tmpDir := t.TempDir()
	path = filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	state = NewState()
	// Must call executeRead before edit to register the file's modification time.
	// The edit operation validates that the file hasn't been externally modified since this read.
	_, err := state.executeRead(context.Background(), path, 0, 0)
	require.NoError(t, err)
	return state, path
}

func callEdit(t *testing.T, state *State, input EditInput) (string, error) {
	t.Helper()
	return state.executeEdit(context.Background(), input.FilePath, input.OldString, input.NewString, input.ReplaceAll)
}

func TestEdit_BasicFunctionality(t *testing.T) {
	t.Run("simple replacement", func(t *testing.T) {
		state, path := setupFileForEdit(t, "Hello World")
		result, err := callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "World",
			NewString: "Universe",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "has been updated")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "Hello Universe", string(content))
	})
	t.Run("multiline replacement", func(t *testing.T) {
		state, path := setupFileForEdit(t, "Line 1\nLine 2\nLine 3")
		result, err := callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "Line 2",
			NewString: "Modified Line 2",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "has been updated")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "Line 1\nModified Line 2\nLine 3", string(content))
	})
	t.Run("replace all occurrences", func(t *testing.T) {
		state, path := setupFileForEdit(t, "foo bar foo baz foo")
		result, err := callEdit(t, state, EditInput{
			FilePath:   path,
			OldString:  "foo",
			NewString:  "FOO",
			ReplaceAll: true,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "All occurrences")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "FOO bar FOO baz FOO", string(content))
	})
}

func TestEdit_Errors(t *testing.T) {
	state := NewState()
	t.Run("string not found", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))
		_, err := state.executeRead(context.Background(), path, 0, 0)
		require.NoError(t, err)
		_, err = callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "nonexistent",
			NewString: "replacement",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
	t.Run("old and new strings are same", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))
		_, err := state.executeRead(context.Background(), path, 0, 0)
		require.NoError(t, err)
		_, err = callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "same",
			NewString: "same",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "same")
	})
	t.Run("multiple matches without replace_all", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("foo foo foo"), 0o644))
		_, err := state.executeRead(context.Background(), path, 0, 0)
		require.NoError(t, err)
		_, err = callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "foo",
			NewString: "bar",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Found 3 matches")
		assert.Contains(t, err.Error(), "replace_all")
	})
	t.Run("file not read before edit", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))
		_, err := callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "content",
			NewString: "new",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read")
	})
	t.Run("file modified after read", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))
		_, err := state.executeRead(context.Background(), path, 0, 0)
		require.NoError(t, err)
		// Sleep ensures the file's modification time will be strictly after the read operation's timestamp.
		// This prevents false negatives due to filesystem timestamp granularity.
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, os.WriteFile(path, []byte("modified externally"), 0o644))
		_, err = callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "original",
			NewString: "new",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "modified since")
	})
	t.Run("relative path rejected", func(t *testing.T) {
		_, err := callEdit(t, state, EditInput{
			FilePath:  "./test.txt",
			OldString: "old",
			NewString: "new",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be absolute")
	})
}

func TestEdit_AdvancedScenarios(t *testing.T) {
	t.Run("whitespace preservation", func(t *testing.T) {
		state, path := setupFileForEdit(t, "  indented line\n    more indented")
		result, err := callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "  indented line",
			NewString: "  modified line",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "has been updated")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "  modified line\n    more indented", string(content))
	})
	t.Run("special characters", func(t *testing.T) {
		state, path := setupFileForEdit(t, "Hello \"World\" & 'Universe'")
		result, err := callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "\"World\"",
			NewString: "\"Galaxy\"",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "has been updated")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "Hello \"Galaxy\" & 'Universe'", string(content))
	})
	t.Run("empty string replacement", func(t *testing.T) {
		state, path := setupFileForEdit(t, "remove this word from sentence")
		result, err := callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "word ",
			NewString: "",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "has been updated")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "remove this from sentence", string(content))
	})
	t.Run("large replacement", func(t *testing.T) {
		state, path := setupFileForEdit(t, "short")
		replacement := strings.Repeat("long ", 100)
		result, err := callEdit(t, state, EditInput{
			FilePath:  path,
			OldString: "short",
			NewString: replacement,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "has been updated")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, replacement, string(content))
	})
}

func TestEdit_MCPIntegration(t *testing.T) {
	// Tests the public MCP (Model Context Protocol) API functions, which wrap the underlying executeRead
	// and executeEdit methods and return SDK-compatible responses.
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("original content"), 0o644))
	_, _, err := Read(context.Background(), &sdk.CallToolRequest{}, ReadInput{FilePath: path})
	require.NoError(t, err)
	result, _, err := Edit(context.Background(), &sdk.CallToolRequest{}, EditInput{
		FilePath:  path,
		OldString: "original",
		NewString: "modified",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "modified content", string(content))
}

func TestModifiedLines(t *testing.T) {
	tests := []struct {
		name      string
		oldLines  []string
		newLines  []string
		delta     int
		wantStart int
		wantEnd   int
	}{
		{
			name:      "single line changed",
			oldLines:  []string{"line1", "line2", "line3", "line4", "line5"},
			newLines:  []string{"line1", "CHANGED", "line3", "line4", "line5"},
			delta:     1,
			wantStart: 1,
			wantEnd:   3,
		},
		{
			name:      "multiple lines changed",
			oldLines:  []string{"line1", "line2", "line3", "line4", "line5"},
			newLines:  []string{"line1", "CHANGED1", "CHANGED2", "line4", "line5"},
			delta:     1,
			wantStart: 1,
			wantEnd:   4,
		},
		{
			name:      "no context",
			oldLines:  []string{"line1", "line2", "line3"},
			newLines:  []string{"line1", "CHANGED", "line3"},
			delta:     0,
			wantStart: 2,
			wantEnd:   2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// delta controls how many context lines to include around the modified lines.
			// When delta=1, each line of context is expanded by 1 line above/below the change.
			// When delta=0, only the exact changed lines are returned.
			start, end := modifiedLines(tt.oldLines, tt.newLines, tt.delta)
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
		})
	}
}
