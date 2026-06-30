package zhihu

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientHotListParsesItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/topstory/hot-list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "2" {
			t.Fatalf("limit = %q, want 2", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": [
				{
					"target": {
						"title": "First question",
						"excerpt": "short summary",
						"url": "https://api.zhihu.com/questions/123",
						"question": {"id": 123}
					},
					"detail_text": "100 万热度"
				},
				{
					"target": {
						"title": "Second question",
						"excerpt": "",
						"url": "https://api.zhihu.com/questions/456"
					},
					"detail_text": "50 万热度"
				},
				{
					"target": {
						"title": "Third question",
						"url": "https://api.zhihu.com/questions/789"
					},
					"detail_text": "10 万热度"
				}
			]
		}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL))
	items, err := client.HotList(t.Context(), 2)
	if err != nil {
		t.Fatalf("HotList returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Title != "First question" || items[0].QuestionID != 123 {
		t.Fatalf("first item = %#v", items[0])
	}
	if items[0].URL != "https://www.zhihu.com/question/123" {
		t.Fatalf("first URL = %q", items[0].URL)
	}
	if items[1].QuestionID != 456 {
		t.Fatalf("second question id = %d, want 456", items[1].QuestionID)
	}
	if items[1].URL != "https://www.zhihu.com/question/456" {
		t.Fatalf("second URL = %q", items[1].URL)
	}
}

func TestClientSendsConfiguredCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != "SESSION=abc" {
			t.Fatalf("Cookie = %q, want SESSION=abc", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	client := NewClient(WithBaseURL(server.URL), WithCookie("SESSION=abc"))
	if _, err := client.HotList(t.Context(), 1); err != nil {
		t.Fatalf("HotList returned error: %v", err)
	}
}
