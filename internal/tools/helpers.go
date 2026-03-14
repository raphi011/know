package tools

import (
	"encoding/json"
	"fmt"
)

func parseInput[T any](argumentsInJSON string, toolName string) (*T, error) {
	var input T
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		return nil, fmt.Errorf("parse %s input: %w", toolName, err)
	}
	return &input, nil
}
