package zhihu

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeSession struct {
	fetchPath   string
	fetchParams map[string]string
	postPath    string
	postBody    any
	postCalled  bool
	requests    []fakeRequest
	loginStatus LoginStatus
}

type fakeRequest struct {
	method string
	path   string
	body   any
}

func (f *fakeSession) FetchJSON(_ context.Context, path string, params map[string]string, target any) error {
	f.fetchPath = path
	f.fetchParams = params
	switch path {
	case "/topstory/hot-list":
		return json.Unmarshal([]byte(`{
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
		}`), target)
	default:
		return json.Unmarshal([]byte(`{}`), target)
	}
}

func (f *fakeSession) PostJSON(_ context.Context, path string, body any, target any) error {
	f.postCalled = true
	f.postPath = path
	f.postBody = body
	return json.Unmarshal([]byte(`{"id":"456","url":"https://api.zhihu.com/answers/456"}`), target)
}

func (f *fakeSession) RequestJSON(_ context.Context, method string, path string, body any, target any) error {
	f.requests = append(f.requests, fakeRequest{method: method, path: path, body: body})
	switch {
	case method == "POST" && path == "https://zhuanlan.zhihu.com/api/articles/drafts":
		return json.Unmarshal([]byte(`{"id":"789","url":"https://zhuanlan.zhihu.com/p/789"}`), target)
	case method == "PATCH" && path == "https://zhuanlan.zhihu.com/api/articles/789/draft":
		return json.Unmarshal([]byte(`{"id":"789","url":"https://zhuanlan.zhihu.com/p/789"}`), target)
	case method == "POST" && path == "https://www.zhihu.com/api/v4/content/publish":
		action := ""
		if m, ok := body.(map[string]any); ok {
			action, _ = m["action"].(string)
		}
		if action == "update" {
			return json.Unmarshal([]byte(`{"data":{"result":"{\"id\":\"456\",\"url\":\"https://zhuanlan.zhihu.com/p/456\"}"}}`), target)
		}
		return json.Unmarshal([]byte(`{"data":{"result":"{\"id\":\"789\",\"url\":\"https://zhuanlan.zhihu.com/p/789\"}"}}`), target)
	default:
		return json.Unmarshal([]byte(`{}`), target)
	}
}

func (f *fakeSession) OpenLogin(context.Context) (LoginResult, error) {
	return LoginResult{LoginURL: "https://www.zhihu.com/signin", ProfileDir: ".zhihu-profile", Message: "opened"}, nil
}

func (f *fakeSession) LoginStatus(context.Context) (LoginStatus, error) {
	return f.loginStatus, nil
}

func (f *fakeSession) Close() error {
	return nil
}

func TestClientHotListParsesItemsFromBrowserSession(t *testing.T) {
	session := &fakeSession{}
	client := NewClient(WithSession(session))
	items, err := client.HotList(t.Context(), 2)
	if err != nil {
		t.Fatalf("HotList returned error: %v", err)
	}
	if session.fetchPath != "/topstory/hot-list" {
		t.Fatalf("fetch path = %q", session.fetchPath)
	}
	if session.fetchParams["limit"] != "2" {
		t.Fatalf("limit = %q, want 2", session.fetchParams["limit"])
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
	if items[1].QuestionID != 456 || items[1].URL != "https://www.zhihu.com/question/456" {
		t.Fatalf("second item = %#v", items[1])
	}
}

func TestPublishAnswerDryRunDoesNotCallBrowserPost(t *testing.T) {
	session := &fakeSession{}
	client := NewClient(WithSession(session))
	result, err := client.PublishAnswer(t.Context(), PublishAnswerRequest{
		QuestionID: 123,
		Content:    "  hello zhihu  ",
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("PublishAnswer returned error: %v", err)
	}
	if session.postCalled {
		t.Fatal("PostJSON was called during dry run")
	}
	if !result.DryRun || result.QuestionID != 123 || result.Content != "hello zhihu" {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
}

func TestPublishAnswerPostsThroughBrowserSession(t *testing.T) {
	session := &fakeSession{}
	client := NewClient(WithSession(session))
	result, err := client.PublishAnswer(t.Context(), PublishAnswerRequest{
		QuestionID: 123,
		Content:    "hello zhihu",
	})
	if err != nil {
		t.Fatalf("PublishAnswer returned error: %v", err)
	}
	if !session.postCalled {
		t.Fatal("PostJSON was not called")
	}
	if session.postPath != "https://www.zhihu.com/api/v4/questions/123/answers" {
		t.Fatalf("post path = %q", session.postPath)
	}
	body := session.postBody.(map[string]any)
	if body["content"] != "<p>hello zhihu</p>" {
		t.Fatalf("content = %#v", body["content"])
	}
	if body["reshipment_settings"] != "allowed" || body["comment_permission"] != "all" {
		t.Fatalf("missing publish settings: %#v", body)
	}
	reward := body["reward_setting"].(map[string]any)
	if reward["can_reward"] != false {
		t.Fatalf("reward_setting = %#v", reward)
	}
	if result.DryRun || result.AnswerID != 456 || result.URL != "https://www.zhihu.com/question/123/answer/456" {
		t.Fatalf("unexpected publish result: %#v", result)
	}
}

func TestPublishArticleDryRunDoesNotCallBrowserRequest(t *testing.T) {
	session := &fakeSession{}
	client := NewClient(WithSession(session))
	result, err := client.PublishArticle(t.Context(), PublishArticleRequest{
		Title:   "  Slidr Free  ",
		Content: "  hello article  ",
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("PublishArticle returned error: %v", err)
	}
	if len(session.requests) != 0 {
		t.Fatal("RequestJSON was called during dry run")
	}
	if !result.DryRun || result.Title != "Slidr Free" || result.Content != "hello article" {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
}

func TestPublishArticleCreatesUpdatesAndPublishesDraft(t *testing.T) {
	session := &fakeSession{}
	client := NewClient(WithSession(session))
	result, err := client.PublishArticle(t.Context(), PublishArticleRequest{
		Title:   "Slidr Free",
		Content: "First line\n\nhttps://github.com/YuriGao/slidr-free",
	})
	if err != nil {
		t.Fatalf("PublishArticle returned error: %v", err)
	}
	if len(session.requests) != 3 {
		t.Fatalf("request count = %d, want 3", len(session.requests))
	}
	want := []fakeRequest{
		{method: "POST", path: "https://zhuanlan.zhihu.com/api/articles/drafts"},
		{method: "PATCH", path: "https://zhuanlan.zhihu.com/api/articles/789/draft"},
		{method: "POST", path: "https://www.zhihu.com/api/v4/content/publish"},
	}
	for i := range want {
		if session.requests[i].method != want[i].method || session.requests[i].path != want[i].path {
			t.Fatalf("request %d = %#v, want %#v", i, session.requests[i], want[i])
		}
	}
	patchBody := session.requests[1].body.(map[string]any)
	if patchBody["title"] != "Slidr Free" {
		t.Fatalf("title = %#v", patchBody["title"])
	}
	if !strings.Contains(patchBody["content"].(string), "<p>First line</p>") {
		t.Fatalf("content html = %q", patchBody["content"])
	}
	publishBody := session.requests[2].body.(map[string]any)
	if publishBody["action"] != "publish" {
		t.Fatalf("publish action = %#v", publishBody["action"])
	}
	data := publishBody["data"].(map[string]any)
	if data["type"] != "article" || data["article_id"] != int64(789) {
		t.Fatalf("publish data = %#v", data)
	}
	if result.DryRun || result.ArticleID != 789 || result.URL != "https://zhuanlan.zhihu.com/p/789" {
		t.Fatalf("unexpected publish result: %#v", result)
	}
}

func TestPublishArticleUsesProvidedHTMLContent(t *testing.T) {
	session := &fakeSession{}
	client := NewClient(WithSession(session))
	_, err := client.PublishArticle(t.Context(), PublishArticleRequest{
		Title:       "Slidr Free",
		Content:     "fallback plain",
		ContentHTML: `<h2>项目亮点</h2><ul><li>边缘滑动</li></ul>`,
	})
	if err != nil {
		t.Fatalf("PublishArticle returned error: %v", err)
	}
	if len(session.requests) != 3 {
		t.Fatalf("request count = %d, want 3", len(session.requests))
	}
	patchBody := session.requests[1].body.(map[string]any)
	if patchBody["content"] != `<h2>项目亮点</h2><ul><li>边缘滑动</li></ul>` {
		t.Fatalf("draft content = %#v", patchBody["content"])
	}
	publishBody := session.requests[2].body.(map[string]any)
	data := publishBody["data"].(map[string]any)
	if data["content"] != `<h2>项目亮点</h2><ul><li>边缘滑动</li></ul>` {
		t.Fatalf("publish content = %#v", data["content"])
	}
}

func TestUpdateAnswerRejectsUnsafeNonDryRunWithoutNetworkRequest(t *testing.T) {
	session := &fakeSession{}
	client := NewClient(WithSession(session))
	_, err := client.UpdateAnswer(t.Context(), UpdateAnswerRequest{
		QuestionID:  123,
		AnswerID:    456,
		Content:     "fallback plain",
		ContentHTML: `<h2>项目亮点</h2><ul><li>边缘滑动</li></ul>`,
	})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("UpdateAnswer error = %v, want unsupported error", err)
	}
	if len(session.requests) != 0 || session.postCalled {
		t.Fatalf("unsafe update made network calls: postCalled=%v requests=%#v", session.postCalled, session.requests)
	}
}

func TestUpdateArticleRejectsUnsafeNonDryRunWithoutNetworkRequest(t *testing.T) {
	session := &fakeSession{}
	client := NewClient(WithSession(session))
	_, err := client.UpdateArticle(t.Context(), UpdateArticleRequest{
		ArticleID:   456,
		Title:       "Slidr Free",
		Content:     "fallback plain",
		ContentHTML: `<h2>安装</h2><pre><code>swift build</code></pre>`,
	})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("UpdateArticle error = %v, want unsupported error", err)
	}
	if len(session.requests) != 0 || session.postCalled {
		t.Fatalf("unsafe update made network calls: postCalled=%v requests=%#v", session.postCalled, session.requests)
	}
}

func TestLoginStatusUsesPersistentProfile(t *testing.T) {
	session := &fakeSession{loginStatus: LoginStatus{
		LoggedIn:   true,
		ProfileDir: ".zhihu-profile",
		Message:    "logged in",
	}}
	client := NewClient(WithSession(session))
	status, err := client.LoginStatus(t.Context())
	if err != nil {
		t.Fatalf("LoginStatus returned error: %v", err)
	}
	if !status.LoggedIn || !strings.Contains(status.ProfileDir, ".zhihu-profile") {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestInt64FromAnyPreservesJSONNumberIDs(t *testing.T) {
	id := int64FromAny(json.Number("2055401990973400552"))
	if id != 2055401990973400552 {
		t.Fatalf("id = %d, want exact Zhihu id", id)
	}
}
