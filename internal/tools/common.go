package tools

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// resolvePath validates and normalizes a file path. It rejects relative paths to prevent
// directory traversal attacks and ensures all file operations use absolute, canonical paths.
func resolvePath(filePath string) (string, error) {
	if !filepath.IsAbs(filePath) {
		return "", fmt.Errorf("file path must be absolute, not relative")
	}
	return filepath.Clean(filePath), nil
}

// catN formats lines with line numbers in the style of `cat -n`, using a dynamically-sized
// column width to align numbers. This ensures proper alignment even for files with thousands
// of lines. Each line is truncated to 2000 characters to prevent excessively large output.
func catN(lines []string, startLine int) string {
	if len(lines) == 0 {
		return ""
	}
	var formattedLines []string
	// Calculate formatter width based on the maximum line number. Ensures a minimum width of 6
	// to match `cat -n` conventions, but expands for files with more than 999,999 lines.
	lineNumFormatter := "%" + fmt.Sprintf("%dd", max(6, len(strconv.Itoa(startLine+len(lines)))))
	for i, line := range lines {
		lineNum := startLine + i
		if len(line) > 2000 {
			line = line[:2000]
		}
		formattedLines = append(formattedLines, fmt.Sprintf(lineNumFormatter+"→%s", lineNum, line))
	}
	return strings.Join(formattedLines, "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// modifiedLines calculates the range of modified lines between two file versions.
// It finds the first and last lines that differ, then expands the range by `delta`
// lines on each side to provide context for the user. This is used by the edit tool
// to show surrounding context around the change. Line numbers are 1-indexed.
//
// The algorithm scans from both ends simultaneously:
// - i: index of first differing line (from start)
// - j: count of matching lines from the end
// The delta parameter allows showing context lines before and after the actual changes.
func modifiedLines(oldLines, newLines []string, delta int) (start, end int) {
	if delta < 0 {
		delta = 0
	}
	// Scan forward from the beginning to find the first differing line.
	i := 0
	for i < len(oldLines) && i < len(newLines) && oldLines[i] == newLines[i] {
		i++
	}
	// Scan backward from the end to find the last differing line.
	j := 0
	for (len(oldLines)-1-j) >= i && (len(newLines)-1-j) >= i &&
		oldLines[len(oldLines)-1-j] == newLines[len(newLines)-1-j] {
		j++
	}
	start = i + 1
	end = len(oldLines) - j
	lo := 1
	hi := len(oldLines)
	// Expand the range by delta lines on each side to show context.
	start -= delta
	if start < lo {
		start = lo
	}
	end += delta
	if end > hi {
		end = hi
	}
	return start, end
}
