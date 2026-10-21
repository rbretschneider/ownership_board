package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Tool is one MCP tool. Add a capability by adding a Tool; the server does not
// change.
type Tool interface {
	Definition() Definition
	// Call runs the tool and returns text for the model.
	Call(ctx context.Context, actor domain.Actor, arguments json.RawMessage) (string, error)
}

// Definition is how a tool is listed to the client.
type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ArgumentError means the model sent arguments the tool cannot use.
type ArgumentError struct {
	Message string
}

func (e *ArgumentError) Error() string {
	return e.Message
}

// decodeArguments parses arguments strictly, so a misspelt field is reported
// instead of silently ignored.
func decodeArguments(raw json.RawMessage, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return &ArgumentError{Message: fmt.Sprintf("invalid arguments: %v", err)}
	}
	return nil
}

// object builds a JSON Schema object with the given properties.
func object(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func prop(kind, description string) map[string]any {
	return map[string]any{"type": kind, "description": description}
}

func ticketIDProp() map[string]any {
	return prop("integer", "The ticket id, as in #42.")
}
