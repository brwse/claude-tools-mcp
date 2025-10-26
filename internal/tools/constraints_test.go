package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCatN(t *testing.T) {
	tests := []struct {
		name      string
		lines     []string
		startLine int
		want      string
	}{
		{
			name:      "simple lines",
			lines:     []string{"line1", "line2", "line3"},
			startLine: 1,
			// catN dynamically pads line numbers to align with the highest line number
			// (max line 3 uses 1-char width, which explains the 5 leading spaces for alignment).
			want: "     1→line1\n     2→line2\n     3→line3",
		},
		{
			name:      "starting at different line",
			lines:     []string{"lineA", "lineB"},
			startLine: 10,
			// Starting at line 10 with 2 lines means max line is 11 (2-char width),
			// so all line numbers are padded to match that width.
			want: "    10→lineA\n    11→lineB",
		},
		{
			name:      "empty lines",
			lines:     []string{},
			startLine: 1,
			want:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := catN(tt.lines, tt.startLine)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"first smaller", 1, 2, 1},
		{"second smaller", 5, 3, 3},
		{"negative numbers", -1, 0, -1},
		{"equal values", 10, 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := min(tt.a, tt.b)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"second larger", 1, 2, 2},
		{"first larger", 5, 3, 5},
		{"with negative", -1, 0, 0},
		{"equal values", 10, 10, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := max(tt.a, tt.b)
			assert.Equal(t, tt.want, result)
		})
	}
}
