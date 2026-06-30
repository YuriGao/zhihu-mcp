package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/YuriGao/zhihu-mcp/internal/zhihu"
)

type ZhihuClient interface {
	HotList(context.Context, int) ([]zhihu.HotItem, error)
	Search(context.Context, string, int) ([]zhihu.SearchItem, error)
	Question(context.Context, int64) (zhihu.Question, error)
	Answers(context.Context, int64, int) ([]zhihu.Answer, error)
}

type Server struct {
	zhihu ZhihuClient
}

func NewServer(zhihuClient ZhihuClient) *Server {
	return &Server{zhihu: zhihuClient}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	writer := bufio.NewWriter(out)
	defer writer.Flush()

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			if err := writeResponse(writer, response{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &rpcError{Code: -32700, Message: "parse error"},
			}); err != nil {
				return err
			}
			continue
		}
		resp, ok := s.handle(ctx, req)
		if !ok {
			continue
		}
		if err := writeResponse(writer, resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) handle(ctx context.Context, req request) (response, bool) {
	if req.ID == nil {
		return response{}, false
	}
	result, err := s.dispatch(ctx, req)
	if err != nil {
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32603, Message: err.Error()}}, true
	}
	return response{JSONRPC: "2.0", ID: req.ID, Result: result}, true
}

func (s *Server) dispatch(ctx context.Context, req request) (any, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "zhihu-mcp",
				"version": "0.1.0",
			},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": tools()}, nil
	case "tools/call":
		return s.callTool(ctx, req.Params)
	default:
		return nil, fmt.Errorf("method not found: %s", req.Method)
	}
}

func (s *Server) callTool(ctx context.Context, params json.RawMessage) (any, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}
	switch call.Name {
	case "zhihu_hot_list":
		var args struct {
			Limit int `json:"limit"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		return textResult(s.zhihu.HotList(ctx, args.Limit))
	case "zhihu_search":
		var args struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		return textResult(s.zhihu.Search(ctx, args.Query, args.Limit))
	case "zhihu_question":
		var args struct {
			QuestionID int64 `json:"question_id"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		return textResult(s.zhihu.Question(ctx, args.QuestionID))
	case "zhihu_answers":
		var args struct {
			QuestionID int64 `json:"question_id"`
			Limit      int   `json:"limit"`
		}
		_ = json.Unmarshal(call.Arguments, &args)
		return textResult(s.zhihu.Answers(ctx, args.QuestionID, args.Limit))
	default:
		return nil, fmt.Errorf("unknown tool: %s", call.Name)
	}
}

func textResult(value any, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"content": []toolContent{{
			Type: "text",
			Text: string(data),
		}},
	}, nil
}

func writeResponse(w *bufio.Writer, resp response) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return w.Flush()
}

func tools() []map[string]any {
	return []map[string]any{
		{
			"name":        "zhihu_hot_list",
			"description": "Get current Zhihu hot list items.",
			"inputSchema": objectSchema(map[string]any{
				"limit": numberSchema("Maximum number of items, default 10, max 50."),
			}, []string{}),
		},
		{
			"name":        "zhihu_search",
			"description": "Search Zhihu content by keyword.",
			"inputSchema": objectSchema(map[string]any{
				"query": stringSchema("Search keyword."),
				"limit": numberSchema("Maximum number of results, default 10, max 20."),
			}, []string{"query"}),
		},
		{
			"name":        "zhihu_question",
			"description": "Get metadata for a Zhihu question.",
			"inputSchema": objectSchema(map[string]any{
				"question_id": numberSchema("Zhihu question ID."),
			}, []string{"question_id"}),
		},
		{
			"name":        "zhihu_answers",
			"description": "Get answers for a Zhihu question.",
			"inputSchema": objectSchema(map[string]any{
				"question_id": numberSchema("Zhihu question ID."),
				"limit":       numberSchema("Maximum number of answers, default 5, max 20."),
			}, []string{"question_id"}),
		},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func numberSchema(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}
