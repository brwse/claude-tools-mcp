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

func callWrite(t *testing.T, state *State, input WriteInput) (string, error) {
	t.Helper()
	return state.executeWrite(context.Background(), input.FilePath, input.Content)
}

func TestWrite_BasicFunctionality(t *testing.T) {
	state := NewState()
	t.Run("create new file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "new_file.txt")
		result, err := callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  "Hello, World!",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "created successfully")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "Hello, World!", string(content))
	})
	t.Run("overwrite existing file after read", func(t *testing.T) {
		// The Write tool enforces a safety guard: existing files can only be overwritten
		// after explicitly being read first. This ensures the caller has seen the current
		// content before modifying it, preventing accidental overwrites of unexpected data.
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "existing.txt")
		require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))
		_, err := state.executeRead(context.Background(), path, 0, 0)
		require.NoError(t, err)
		result, err := callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  "updated",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "updated successfully")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "updated", string(content))
	})
	t.Run("creates parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "subdir1", "subdir2", "file.txt")
		result, err := callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  "nested content",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "created successfully")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "nested content", string(content))
	})
	t.Run("empty content", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "empty.txt")
		result, err := callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  "",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "created successfully")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "", string(content))
	})
}

func TestWrite_Errors(t *testing.T) {
	state := NewState()
	t.Run("existing file without prior read", func(t *testing.T) {
		// Validates the guard mechanism: writing to an existing file that hasn't been
		// explicitly read triggers an error. This prevents silent overwrites of files
		// that may have been modified by external processes since the tool was invoked.
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "existing.txt")
		require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))
		_, err := callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  "new content",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read it first")
	})
	t.Run("file modified after read", func(t *testing.T) {
		// Detects external modifications: the Write tool tracks file modification times
		// during Read operations. If a file's mtime changes between being read and written,
		// the write is rejected. This guards against race conditions where external processes
		// (version control, editors, other tools) modify the file without the tool's knowledge.
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))
		_, err := state.executeRead(context.Background(), path, 0, 0)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, os.WriteFile(path, []byte("externally modified"), 0o644))
		_, err = callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  "new content",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "modified since")
	})
	t.Run("relative path rejected", func(t *testing.T) {
		// Enforces absolute paths: relative paths are rejected to prevent ambiguity
		// about the working directory context. This ensures predictable behavior
		// regardless of how the tool is invoked.
		_, err := callWrite(t, state, WriteInput{
			FilePath: "./relative/path.txt",
			Content:  "content",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be absolute")
	})
}

func TestWrite_AdvancedScenarios(t *testing.T) {
	// Tests edge cases and robustness: these scenarios verify that the Write tool
	// handles diverse content types and path structures correctly, ensuring that
	// the guard mechanism and path validation don't interfere with legitimate operations.
	state := NewState()
	t.Run("special characters", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "special.txt")
		specialContent := "Hello 世界 🌍\n\"quoted\"\n'single'\n\ttabbed"
		result, err := callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  specialContent,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "created successfully")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, specialContent, string(content))
	})
	t.Run("large file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "large.txt")
		largeContent := strings.Repeat("line\n", 10000)
		result, err := callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  largeContent,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "created successfully")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, largeContent, string(content))
	})
	t.Run("binary content", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "binary.dat")
		binaryContent := string([]byte{0x00, 0x01, 0xFF, 0xFE})
		result, err := callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  binaryContent,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "created successfully")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, binaryContent, string(content))
	})
	t.Run("path with spaces", func(t *testing.T) {
		tmpDir := t.TempDir()
		subDir := filepath.Join(tmpDir, "dir with spaces")
		require.NoError(t, os.MkdirAll(subDir, 0o755))
		path := filepath.Join(subDir, "file with spaces.txt")
		result, err := callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  "content",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "created successfully")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "content", string(content))
	})
	t.Run("multiple writes with reads", func(t *testing.T) {
		// Validates the state tracking pattern: files can be created without prior reads,
		// but subsequent updates require an intervening read. This allows clean creation
		// but maintains safety for modifications by tracking the file's mtime.
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "multi.txt")
		result, err := callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  "first",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "created successfully")
		_, err = state.executeRead(context.Background(), path, 0, 0)
		require.NoError(t, err)
		result, err = callWrite(t, state, WriteInput{
			FilePath: path,
			Content:  "second",
		})
		require.NoError(t, err)
		assert.Contains(t, result, "updated successfully")
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "second", string(content))
	})
}

func TestWrite_MCPIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	result, _, err := Write(context.Background(), &sdk.CallToolRequest{}, WriteInput{
		FilePath: path,
		Content:  "test content",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))
}
