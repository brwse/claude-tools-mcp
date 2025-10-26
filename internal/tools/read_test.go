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

func setupTestFile(t *testing.T, content string) (state *State, path string) {
	t.Helper()
	tmpDir := t.TempDir()
	path = filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return NewState(), path
}

func callRead(t *testing.T, state *State, input ReadInput) (string, error) {
	t.Helper()
	result, err := state.executeRead(context.Background(), input.FilePath, input.Offset, input.Limit)
	return result, err
}

func TestRead_BasicFunctionality(t *testing.T) {
	t.Run("simple file", func(t *testing.T) {
		state, path := setupTestFile(t, "Line 1\nLine 2\nLine 3")
		result, err := callRead(t, state, ReadInput{FilePath: path})
		require.NoError(t, err)
		expected := "     1→Line 1\n     2→Line 2\n     3→Line 3"
		assert.Equal(t, expected, result)
	})
	t.Run("empty file shows warning", func(t *testing.T) {
		state, path := setupTestFile(t, "")
		result, err := callRead(t, state, ReadInput{FilePath: path})
		require.NoError(t, err)
		assert.Contains(t, result, "<system-reminder>")
		assert.Contains(t, result, "empty")
	})
	t.Run("tracks read time for edit validation", func(t *testing.T) {
		// ReadFiles tracking is used by the Edit tool to detect if a file has been
		// modified since it was read, preventing accidental overwrites of concurrent changes.
		state, path := setupTestFile(t, "test content")
		_, err := callRead(t, state, ReadInput{FilePath: path})
		require.NoError(t, err)
		state.Mu.Lock()
		readTime, exists := state.ReadFiles[path]
		state.Mu.Unlock()
		require.True(t, exists)
		fileInfo, err := os.Stat(path)
		require.NoError(t, err)
		assert.True(t, readTime.Equal(fileInfo.ModTime()))
	})
}

func TestRead_OffsetAndLimit(t *testing.T) {
	tests := []struct {
		name           string
		numLines       int
		offset         int64
		limit          int64
		wantLines      int
		wantFirst      string
		wantLast       string
		wantNotContain string
	}{
		{"offset only", 10, 5, 0, 6, "     5→", "    10→", "1→"},
		{"limit only", 10, 0, 3, 3, "     1→", "     3→", "4→"},
		{"both", 20, 10, 5, 5, "    10→", "    14→", "15→"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := NewState()
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "test.txt")
			lines := make([]string, tt.numLines)
			for i := range tt.numLines {
				lines[i] = "Line " + string(rune('A'+i))
			}
			require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644))
			result, err := callRead(t, state, ReadInput{
				FilePath: path,
				Offset:   tt.offset,
				Limit:    tt.limit,
			})
			require.NoError(t, err)
			resultLines := strings.Split(result, "\n")
			assert.Len(t, resultLines, tt.wantLines)
			assert.True(t, strings.HasPrefix(resultLines[0], tt.wantFirst))
			assert.True(t, strings.HasPrefix(resultLines[len(resultLines)-1], tt.wantLast))
			assert.NotContains(t, result, tt.wantNotContain)
		})
	}
}

func TestRead_Errors(t *testing.T) {
	state := NewState()
	t.Run("non-existent file", func(t *testing.T) {
		_, err := callRead(t, state, ReadInput{
			FilePath: filepath.Join(t.TempDir(), "nonexistent.txt"),
		})
		require.Error(t, err)
	})
	t.Run("relative path rejected", func(t *testing.T) {
		_, err := callRead(t, state, ReadInput{
			FilePath: "./test.txt",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be absolute")
	})
	t.Run("directory rejected", func(t *testing.T) {
		_, err := callRead(t, state, ReadInput{
			FilePath: t.TempDir(),
		})
		require.Error(t, err)
	})
	t.Run("file exceeds 10MB limit", func(t *testing.T) {
		// 10MB limit prevents loading arbitrarily large files that could consume
		// excessive memory and produce output that exceeds the MCP protocol limits.
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "large.txt")
		require.NoError(t, os.WriteFile(path, make([]byte, 11*1024*1024), 0o644))
		_, err := callRead(t, state, ReadInput{FilePath: path})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum allowed size")
	})
}

func TestRead_SizeLimits(t *testing.T) {
	t.Run("large file capped at 2000 lines", func(t *testing.T) {
		// Without offset/limit, large files default to 2000 line cap. This ensures
		// unbounded reads don't produce excessive output that violates MCP constraints.
		state := NewState()
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "large.txt")
		lines := make([]string, 3000)
		for i := range 3000 {
			lines[i] = "line content"
		}
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644))
		result, err := callRead(t, state, ReadInput{FilePath: path})
		require.NoError(t, err)
		resultLines := strings.Split(result, "\n")
		assert.Len(t, resultLines, 2000)
		assert.True(t, strings.HasPrefix(resultLines[0], "     1→"))
		assert.True(t, strings.HasPrefix(resultLines[1999], "  2000→"))
	})
	t.Run("long lines truncated at 2000 chars", func(t *testing.T) {
		// Individual lines longer than 2000 characters are truncated to prevent
		// single pathologically long lines from bloating output size.
		state, path := setupTestFile(t, "short\n"+strings.Repeat("A", 3000)+"\nshort")
		result, err := callRead(t, state, ReadInput{FilePath: path})
		require.NoError(t, err)
		lines := strings.Split(result, "\n")
		var longLine string
		for _, line := range lines {
			if strings.Contains(line, "2→") {
				longLine = line
				break
			}
		}
		assert.LessOrEqual(t, len(longLine), 2010)
	})
	t.Run("offset beyond file returns warning", func(t *testing.T) {
		// When offset exceeds file length, return a warning instead of failing.
		// This allows users to discover the file size without a hard error.
		state, path := setupTestFile(t, "L1\nL2\nL3")
		result, err := callRead(t, state, ReadInput{
			FilePath: path,
			Offset:   100,
		})
		require.NoError(t, err)
		assert.Contains(t, result, "<system-reminder>")
		assert.Contains(t, result, "shorter than the provided offset")
	})
}

func TestRead_BinaryFiles(t *testing.T) {
	t.Run("PNG image returns binary indicator", func(t *testing.T) {
		state := NewState()
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.png")
		pngData := []byte{
			0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
			0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
			0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
			0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
			0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
			0x54, 0x08, 0x99, 0x63, 0xF8, 0x0F, 0x00, 0x00,
			0x01, 0x01, 0x01, 0x00, 0x18, 0xDD, 0x8D, 0xB4,
			0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
			0xAE, 0x42, 0x60, 0x82,
		}
		require.NoError(t, os.WriteFile(path, pngData, 0o644))
		result, err := callRead(t, state, ReadInput{FilePath: path})
		require.NoError(t, err)
		assert.Contains(t, result, "Binary file")
		assert.Contains(t, result, "image/png")
	})
	t.Run("text file returns formatted text", func(t *testing.T) {
		state, path := setupTestFile(t, "plain text")
		result, err := callRead(t, state, ReadInput{FilePath: path})
		require.NoError(t, err)
		assert.Equal(t, "     1→plain text", result)
	})
}

func TestRead_EdgeCases(t *testing.T) {
	t.Run("path with spaces", func(t *testing.T) {
		state := NewState()
		tmpDir := t.TempDir()
		subDir := filepath.Join(tmpDir, "dir with spaces")
		require.NoError(t, os.MkdirAll(subDir, 0o755))
		path := filepath.Join(subDir, "file with spaces.txt")
		require.NoError(t, os.WriteFile(path, []byte("content"), 0o644))
		result, err := callRead(t, state, ReadInput{FilePath: path})
		require.NoError(t, err)
		assert.Equal(t, "     1→content", result)
	})
	t.Run("symlink follows to real file", func(t *testing.T) {
		// Symlinks are followed to their target. This allows reading files specified
		// via symlink paths while still tracking the real file's modification time
		// for edit validation (important for concurrent edit detection).
		state := NewState()
		tmpDir := t.TempDir()
		realFile := filepath.Join(tmpDir, "real.txt")
		require.NoError(t, os.WriteFile(realFile, []byte("content"), 0o644))
		symlinkFile := filepath.Join(tmpDir, "link.txt")
		if err := os.Symlink(realFile, symlinkFile); err != nil {
			t.Skip("symlinks not supported")
		}
		result, err := callRead(t, state, ReadInput{FilePath: symlinkFile})
		require.NoError(t, err)
		assert.Equal(t, "     1→content", result)
	})
	t.Run("multiple reads update tracked time", func(t *testing.T) {
		// Each read updates the tracked modification time. This ensures that Edit
		// validation uses the most recent read timestamp, properly detecting changes
		// made between the original and subsequent reads.
		state, path := setupTestFile(t, "original")
		_, err := callRead(t, state, ReadInput{FilePath: path})
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, os.WriteFile(path, []byte("modified"), 0o644))
		_, err = callRead(t, state, ReadInput{FilePath: path})
		require.NoError(t, err)
		state.Mu.Lock()
		readTime := state.ReadFiles[path]
		state.Mu.Unlock()
		fileInfo, err := os.Stat(path)
		require.NoError(t, err)
		assert.True(t, readTime.Equal(fileInfo.ModTime()))
	})
}

func TestCalculateLineRange(t *testing.T) {
	// calculateLineRange encodes the core logic that bounds output:
	// - No offset/limit on large files (>2000 lines) -> cap at 2000 lines
	// - With limit, never exceed the specified number of lines
	// - Never exceed actual file size
	// This test verifies all edge cases in the clamping logic.
	tests := []struct {
		name       string
		totalLines int
		offset     int
		limit      int
		wantStart  int
		wantEnd    int
	}{
		{"default large file", 3000, 0, 0, 1, 2000},
		{"default small file", 100, 0, 0, 1, 100},
		{"offset only", 100, 50, 0, 50, 100},
		{"limit only", 100, 0, 10, 1, 10},
		{"offset and limit", 100, 20, 15, 20, 34},
		{"limit exceeds file", 100, 90, 20, 90, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := calculateLineRange(tt.totalLines, tt.offset, tt.limit)
			assert.Equal(t, tt.wantStart, start)
			assert.Equal(t, tt.wantEnd, end)
		})
	}
}

func TestRead_MCPIntegration(t *testing.T) {
	// Verify the public Read function (called by the MCP server) properly
	// registers files in the global state for edit validation.
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("test content"), 0o644))
	result, _, err := Read(context.Background(), &sdk.CallToolRequest{}, ReadInput{FilePath: path})
	require.NoError(t, err)
	assert.NotNil(t, result)
	state := GetState()
	state.Mu.Lock()
	_, exists := state.ReadFiles[path]
	state.Mu.Unlock()
	assert.True(t, exists)
}
