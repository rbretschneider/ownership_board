// Package mcp exposes the board to agents as Model Context Protocol tools over
// the streamable HTTP transport. It is stateless and request/response only: no
// sessions and no server-initiated stream.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// Protocol facts.
const (
	jsonRPCVersion        = "2.0"
	latestProtocolVersion = "2025-06-18"
	serverName            = "ownership_board"
	maxRequestBytes       = 1 << 20
)

var supportedProtocolVersions = []string{latestProtocolVersion, "2025-03-26", "2024-11-05"}

// JSON-RPC error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// serverInstructions are shown to the agent when it connects.
const serverInstructions = `This is a task board where you are the worker and a human is the product owner. ` +
	`Call claim_work to get the next ticket; it returns a brief with the request, the history and exact instructions for the action to take. ` +
	`Follow the brief's "What to do now" section. Ask questions with ask_questions instead of guessing; the human answers from a phone. ` +
	`Lanes: new (refine) -> answers (waiting on the human) -> ready -> in_progress -> review (PR open) -> done.`

// Server answers MCP requests by dispatching to its tools.
type Server struct {
	tools   map[string]Tool
	order   []string
	version string
	logger  *slog.Logger
}

// NewServer builds a server offering tools.
func NewServer(version string, logger *slog.Logger, tools ...Tool) *Server {
	server := &Server{tools: map[string]Tool{}, version: version, logger: logger}
	for _, tool := range tools {
		server.tools[tool.Definition().Name] = tool
		server.order = append(server.order, tool.Definition().Name)
	}
	return server
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServeMCP handles one JSON-RPC message from an authenticated agent.
func (s *Server) ServeMCP(w http.ResponseWriter, r *http.Request, actor domain.Actor) {
	var message request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes)).Decode(&message); err != nil {
		writeResponse(w, response{Error: &rpcError{Code: codeParseError, Message: "body is not a JSON-RPC message"}})
		return
	}
	if message.JSONRPC != jsonRPCVersion || message.Method == "" {
		writeResponse(w, response{ID: message.ID, Error: &rpcError{Code: codeInvalidRequest, Message: "not a JSON-RPC 2.0 request"}})
		return
	}
	if len(message.ID) == 0 {
		// A notification needs no reply.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result, failure := s.dispatch(r.Context(), actor, message)
	writeResponse(w, response{ID: message.ID, Result: result, Error: failure})
}

func (s *Server) dispatch(ctx context.Context, actor domain.Actor, message request) (any, *rpcError) {
	switch message.Method {
	case "initialize":
		return s.initialize(message.Params), nil
	case "ping":
		return struct{}{}, nil
	case "tools/list":
		return s.listTools(), nil
	case "tools/call":
		return s.callTool(ctx, actor, message.Params)
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "unknown method " + message.Method}
	}
}

func (s *Server) initialize(params json.RawMessage) any {
	var input struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &input)
	version := latestProtocolVersion
	if slices.Contains(supportedProtocolVersions, input.ProtocolVersion) {
		version = input.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": serverName, "version": s.version},
		"instructions":    serverInstructions,
	}
}

func (s *Server) listTools() any {
	definitions := make([]Definition, 0, len(s.order))
	for _, name := range s.order {
		definitions = append(definitions, s.tools[name].Definition())
	}
	return map[string]any{"tools": definitions}
}

func (s *Server) callTool(ctx context.Context, actor domain.Actor, params json.RawMessage) (any, *rpcError) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "tools/call needs a name and arguments"}
	}
	tool, isKnown := s.tools[call.Name]
	if !isKnown {
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown tool " + call.Name}
	}
	text, err := tool.Call(ctx, actor, defaultArguments(call.Arguments))
	if err != nil {
		return toolError(s.logger, call.Name, err), nil
	}
	return textResult(text, false), nil
}

// toolError reports a failure to the model, which can usually correct its
// call. Only failures that are not the caller's doing are logged.
func toolError(logger *slog.Logger, name string, err error) any {
	var argumentErr *ArgumentError
	if domain.KindOf(err) == 0 && !errors.As(err, &argumentErr) {
		logger.Error("mcp tool failed", "tool", name, "error", err)
		return textResult("The board hit an internal error running "+name+". Try again shortly.", true)
	}
	return textResult(err.Error(), true)
}

func textResult(text string, isError bool) any {
	return map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": isError,
	}
}

func defaultArguments(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("{}")
	}
	return raw
}

func writeResponse(w http.ResponseWriter, body response) {
	body.JSONRPC = jsonRPCVersion
	if body.ID == nil {
		body.ID = json.RawMessage("null")
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
