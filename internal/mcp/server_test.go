package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/YuriGao/zhihu-mcp/internal/zhihu"
)

type fakeZhihu struct{}

func (fakeZhihu) HotList(context.Context, int) ([]zhihu.HotItem, error) {
	return []zhihu.HotItem{{Title: "A", URL: "https://www.zhihu.com/question/1", Heat: "1 万热度", QuestionID: 1}}, nil
}

func (fakeZhihu) Search(context.Context, string, int) ([]zhihu.SearchItem, error) {
	return []zhihu.SearchItem{{Title: "A", URL: "https://www.zhihu.com/question/1", Type: "question"}}, nil
}

func (fakeZhihu) Question(context.Context, int64) (zhihu.Question, error) {
	return zhihu.Question{ID: 1, Title: "A", URL: "https://www.zhihu.com/question/1"}, nil
}

func (fakeZhihu) Answers(context.Context, int64, int) ([]zhihu.Answer, error) {
	return []zhihu.Answer{{ID: 2, Author: "User", Excerpt: "Answer", URL: "https://www.zhihu.com/question/1/answer/2"}}, nil
}

func TestServerHandlesInitializeAndToolCall(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"zhihu_hot_list","arguments":{"limit":1}}}`,
		"",
	}, "\n")
	var out bytes.Buffer

	server := NewServer(fakeZhihu{})
	if err := server.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("response lines = %d, want 2: %s", len(lines), out.String())
	}

	var initResp map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatalf("decode initialize response: %v", err)
	}
	if initResp["id"].(float64) != 1 {
		t.Fatalf("initialize id = %#v", initResp["id"])
	}

	var toolResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &toolResp); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if len(toolResp.Result.Content) != 1 || !strings.Contains(toolResp.Result.Content[0].Text, "zhihu.com/question/1") {
		t.Fatalf("unexpected tool response: %s", lines[1])
	}
}
