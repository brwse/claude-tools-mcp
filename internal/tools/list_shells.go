package tools

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type shellInfo struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type listShellsResult struct {
	Shells []shellInfo `json:"shells"`
	Count  int         `json:"count"`
}

func (s *State) executeListShells(ctx context.Context) (string, error) {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	if len(s.BackgroundShells) == 0 {
		return "No background shells are currently running.", nil
	}

	shells := make([]shellInfo, 0, len(s.BackgroundShells))

	for _, shell := range s.BackgroundShells {
		// Determine status without blocking
		var status string
		select {
		case <-shell.Done:
			if shell.ExitCode != 0 {
				status = "failed"
			} else {
				status = "completed"
			}
		default:
			status = "running"
		}

		info := shellInfo{
			ID:          shell.ID,
			Description: shell.Description,
			Status:      status,
		}
		shells = append(shells, info)
	}

	result := listShellsResult{
		Shells: shells,
		Count:  len(shells),
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("Failed to format shell list: %s", err)
	}

	return string(jsonBytes), nil
}

var ListShellsTool = sdk.Tool{
	Name:        "list_shells",
	Description: "- Lists all background bash shells with their current status\n- Shows shell ID, description, and status (running/completed/failed)\n- Use this tool to see what background shells are active and check their status\n- Useful for tracking long-running operations before fetching their output with bash_output",
}

type ListShellsInput struct {
	// No input parameters needed
}

type ListShellsOutput struct {
	Result string `json:"result"`
}

func ListShells(ctx context.Context, req *sdk.CallToolRequest, args ListShellsInput) (*sdk.CallToolResult, any, error) {
	server := GetState()
	result, err := server.executeListShells(ctx)
	if err != nil {
		return nil, nil, err
	}

	output := &ListShellsOutput{Result: result}
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: result}},
		StructuredContent: output,
	}, output, nil
}
