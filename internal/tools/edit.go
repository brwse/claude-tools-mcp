package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type editItem struct {
	OldString  string
	NewString  string
	ReplaceAll bool
}

func (s *State) executeEdit(ctx context.Context, filePath, oldString, newString string, replaceAll bool) (string, error) {
	edits := []editItem{{OldString: oldString, NewString: newString, ReplaceAll: replaceAll}}
	oldContent, newContent, err := s.applyMultipleEdits(ctx, filePath, edits)
	if err != nil {
		return "", err
	}

	if replaceAll {
		message := fmt.Sprintf(
			"The file %s has been updated. All occurrences of '%s' were successfully replaced with '%s'.",
			filePath,
			oldString,
			newString,
		)
		return message, nil
	}

	// For single replacements, show context around the change so the user can verify the edit was correct
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	start, end := modifiedLines(oldLines, newLines, 2)
	selectedLines := newLines[start:end]
	message := fmt.Sprintf("The file %s has been updated. Here's the result of running `cat -n` on a snippet of the edited file:\n%s", filePath, catN(selectedLines, start))
	return message, nil
}

func validateEdits(edits []editItem) error {
	if len(edits) == 0 {
		return fmt.Errorf("at least one edit is required")
	}
	for _, edit := range edits {
		if edit.OldString == edit.NewString {
			return fmt.Errorf("old_string and new_string are the same - no changes to make")
		}
	}
	return nil
}

func applyEditToContent(content, oldStr, newStr string, replaceAll bool, previousNewStrings []string) (string, error) {
	// When applying sequential edits, detect conflicts where a search string would match part of a previous
	// replacement. This prevents unintended side effects from cascading edits, e.g., if edit 1 replaced "foo"
	// with "foobar" and edit 2 tries to replace "foo", we want to fail rather than silently match the "foo"
	// prefix in "foobar" that wasn't in the original content.
	for _, previousNewString := range previousNewStrings {
		if strings.Contains(previousNewString, oldStr) {
			return "", fmt.Errorf("edit conflict detected: the string to replace is part of a previous edit's replacement")
		}
	}

	count := strings.Count(content, oldStr)
	if count == 0 {
		return "", fmt.Errorf("String to replace not found in file.\nString: %s", oldStr)
	}

	if replaceAll {
		return strings.ReplaceAll(content, oldStr, newStr), nil
	}

	if count > 1 {
		return "", fmt.Errorf(
			"Found %d matches of the string to replace, but replace_all is false. To replace all occurrences, set replace_all to true. To replace only one occurrence, provide more context to uniquely identify the instance.\nString: %s",
			count,
			oldStr,
		)
	}

	return strings.Replace(content, oldStr, newStr, 1), nil
}

func (s *State) applyMultipleEdits(ctx context.Context, filePath string, edits []editItem) (oldContent, newContent string, err error) {
	if err := validateEdits(edits); err != nil {
		return "", "", err
	}
	resolved, err := resolvePath(filePath)
	if err != nil {
		return "", "", err
	}
	if err := s.validateFileForEdit(resolved); err != nil {
		return "", "", err
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", "", fmt.Errorf("Cannot read file: %s", err)
	}
	oldContent = string(content)
	newContent = oldContent
	previousNewStrings := []string{}
	for _, edit := range edits {
		newContent, err = applyEditToContent(newContent, edit.OldString, edit.NewString, edit.ReplaceAll, previousNewStrings)
		if err != nil {
			return oldContent, newContent, err
		}
		previousNewStrings = append(previousNewStrings, edit.NewString)
	}
	if newContent == oldContent {
		return oldContent, newContent, fmt.Errorf("the original content matches the edited content - no changes to make")
	}

	if err = os.WriteFile(resolved, []byte(newContent), 0o600); err != nil {
		return oldContent, newContent, fmt.Errorf("Cannot write file: %s", err)
	}

	// Update the tracked modification time after successful write so that subsequent validateFileForEdit
	// calls won't flag the file as "modified externally". Without this, the next edit would fail because
	// the file's on-disk modTime would be newer than the tracked read time.
	s.Mu.Lock()
	if fileInfo, err := os.Stat(resolved); err == nil {
		s.ReadFiles[resolved] = fileInfo.ModTime()
	}
	s.Mu.Unlock()

	return oldContent, newContent, nil
}

func (s *State) validateFileForEdit(resolved string) error {
	s.Mu.RLock()
	readTime, exists := s.ReadFiles[resolved]
	s.Mu.RUnlock()

	// Require that the file was read before editing to establish a baseline of what the user expects.
	// This ensures the user has visibility into the file content before making string-based replacements,
	// reducing the risk of accidental modifications.
	if !exists {
		return fmt.Errorf("file has not been read yet - please read the file before editing")
	}

	// Detect external modifications to prevent the user's edit from overwriting changes made by other
	// processes. If the file was modified after the last read, the user's search strings may no longer
	// match the expected content, leading to unintended edits.
	fileInfo, err := os.Stat(resolved)
	if err == nil && fileInfo.ModTime().After(readTime) {
		return fmt.Errorf("file has been modified since it was last read - please read the file again before editing")
	}

	return nil
}

var EditTool = sdk.Tool{
	Name:        "edit",
	Description: "Performs exact string replacements in files. \n\nUsage:\n- You must use your `Read` tool at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file. \n- When editing text from Read tool output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the line number prefix. The line number prefix format is: spaces + line number + tab. Everything after that tab is the actual file content to match. Never include any part of the line number prefix in the old_string or new_string.\n- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.\n- Only use emojis if the user explicitly requests it. Avoid adding emojis to files unless asked.\n- The edit will FAIL if `old_string` is not unique in the file. Either provide a larger string with more surrounding context to make it unique or use `replace_all` to change every instance of `old_string`. \n- Use `replace_all` for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance.",
}

type EditInput struct {
	FilePath   string `json:"file_path" jsonschema:"The absolute path to the file to modify"`
	OldString  string `json:"old_string" jsonschema:"The text to replace"`
	NewString  string `json:"new_string" jsonschema:"The text to replace it with (must be different from old_string)"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"Replace all occurrences of old_string (default false)"`
}
type EditOutput struct {
	Message string `json:"message"`
}

func Edit(ctx context.Context, req *sdk.CallToolRequest, args EditInput) (*sdk.CallToolResult, any, error) {
	server := GetState()
	result, err := server.executeEdit(ctx, args.FilePath, args.OldString, args.NewString, args.ReplaceAll)
	if err != nil {
		return nil, nil, err
	}
	output := &EditOutput{Message: result}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: result}},
		StructuredContent: output,
	}, output, nil
}
