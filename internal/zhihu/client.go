package zhihu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const defaultBaseURL = "https://api.zhihu.com"

type browserSession interface {
	FetchJSON(context.Context, string, map[string]string, any) error
	PostJSON(context.Context, string, any, any) error
	RequestJSON(context.Context, string, string, any, any) error
	OpenLogin(context.Context) (LoginResult, error)
	LoginStatus(context.Context) (LoginStatus, error)
	Close() error
}

type Client struct {
	session browserSession
}

type Option func(*clientConfig)

type clientConfig struct {
	baseURL    string
	profileDir string
	headless   bool
	session    browserSession
}

func WithBaseURL(baseURL string) Option {
	return func(c *clientConfig) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

func WithProfileDir(profileDir string) Option {
	return func(c *clientConfig) {
		c.profileDir = profileDir
	}
}

func WithHeadless(headless bool) Option {
	return func(c *clientConfig) {
		c.headless = headless
	}
}

func WithSession(session browserSession) Option {
	return func(c *clientConfig) {
		c.session = session
	}
}

func NewClient(opts ...Option) *Client {
	cfg := clientConfig{
		baseURL:    defaultBaseURL,
		profileDir: defaultProfileDir(),
		headless:   envBool("ZHIHU_HEADLESS", true),
	}
	if profileDir := strings.TrimSpace(os.Getenv("ZHIHU_PROFILE_DIR")); profileDir != "" {
		cfg.profileDir = profileDir
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.session == nil {
		cfg.session = NewPlaywrightSession(PlaywrightSessionConfig{
			BaseURL:    cfg.baseURL,
			ProfileDir: cfg.profileDir,
			Headless:   cfg.headless,
		})
	}
	return &Client{session: cfg.session}
}

type HotItem struct {
	Title      string `json:"title"`
	Excerpt    string `json:"excerpt,omitempty"`
	URL        string `json:"url"`
	Heat       string `json:"heat,omitempty"`
	QuestionID int64  `json:"question_id,omitempty"`
}

type SearchItem struct {
	Title   string `json:"title"`
	Excerpt string `json:"excerpt,omitempty"`
	URL     string `json:"url"`
	Type    string `json:"type,omitempty"`
}

type Question struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Detail      string `json:"detail,omitempty"`
	URL         string `json:"url"`
	AnswerCount int    `json:"answer_count,omitempty"`
	Follower    int    `json:"follower_count,omitempty"`
}

type Answer struct {
	ID      int64  `json:"id"`
	Author  string `json:"author,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
	URL     string `json:"url"`
	Votes   int    `json:"voteup_count,omitempty"`
}

type PublishAnswerRequest struct {
	QuestionID int64  `json:"question_id"`
	Content    string `json:"content"`
	DryRun     bool   `json:"dry_run"`
}

type PublishAnswerResult struct {
	DryRun     bool   `json:"dry_run"`
	QuestionID int64  `json:"question_id"`
	AnswerID   int64  `json:"answer_id,omitempty"`
	URL        string `json:"url,omitempty"`
	Content    string `json:"content"`
	Message    string `json:"message"`
}

type PublishArticleRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	DryRun  bool   `json:"dry_run"`
}

type PublishArticleResult struct {
	DryRun    bool   `json:"dry_run"`
	ArticleID int64  `json:"article_id,omitempty"`
	URL       string `json:"url,omitempty"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Message   string `json:"message"`
}

type UpdateAnswerRequest struct {
	QuestionID  int64  `json:"question_id"`
	AnswerID    int64  `json:"answer_id"`
	Content     string `json:"content"`
	ContentHTML string `json:"content_html,omitempty"`
	DryRun      bool   `json:"dry_run"`
}

type UpdateAnswerResult struct {
	DryRun     bool   `json:"dry_run"`
	QuestionID int64  `json:"question_id"`
	AnswerID   int64  `json:"answer_id"`
	URL        string `json:"url,omitempty"`
	Content    string `json:"content"`
	Message    string `json:"message"`
}

type UpdateArticleRequest struct {
	ArticleID   int64  `json:"article_id"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	ContentHTML string `json:"content_html,omitempty"`
	DryRun      bool   `json:"dry_run"`
}

type UpdateArticleResult struct {
	DryRun    bool   `json:"dry_run"`
	ArticleID int64  `json:"article_id"`
	URL       string `json:"url,omitempty"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Message   string `json:"message"`
}

type LoginResult struct {
	LoginURL   string `json:"login_url"`
	ProfileDir string `json:"profile_dir"`
	Message    string `json:"message"`
}

type LoginStatus struct {
	LoggedIn   bool   `json:"logged_in"`
	ProfileDir string `json:"profile_dir"`
	URL        string `json:"url,omitempty"`
	Message    string `json:"message"`
}

func (c *Client) Close() error {
	return c.session.Close()
}

func (c *Client) OpenLogin(ctx context.Context) (LoginResult, error) {
	return c.session.OpenLogin(ctx)
}

func (c *Client) LoginStatus(ctx context.Context) (LoginStatus, error) {
	return c.session.LoginStatus(ctx)
}

func (c *Client) HotList(ctx context.Context, limit int) ([]HotItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var payload struct {
		Data []struct {
			Target struct {
				Title    string `json:"title"`
				Excerpt  string `json:"excerpt"`
				URL      string `json:"url"`
				Question struct {
					ID int64 `json:"id"`
				} `json:"question"`
			} `json:"target"`
			DetailText string `json:"detail_text"`
		} `json:"data"`
	}
	if err := c.session.FetchJSON(ctx, "/topstory/hot-list", map[string]string{
		"limit": strconv.Itoa(limit),
	}, &payload); err != nil {
		return nil, err
	}
	items := make([]HotItem, 0, len(payload.Data))
	for _, raw := range payload.Data {
		id := raw.Target.Question.ID
		if id == 0 {
			id = questionIDFromURL(raw.Target.URL)
		}
		items = append(items, HotItem{
			Title:      strings.TrimSpace(raw.Target.Title),
			Excerpt:    strings.TrimSpace(raw.Target.Excerpt),
			URL:        questionURL(id, raw.Target.URL),
			Heat:       strings.TrimSpace(raw.DetailText),
			QuestionID: id,
		})
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]SearchItem, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := c.session.FetchJSON(ctx, "/search_v3", map[string]string{
		"t":          "general",
		"q":          query,
		"correction": "1",
		"offset":     "0",
		"limit":      strconv.Itoa(limit),
	}, &payload); err != nil {
		return nil, err
	}
	items := make([]SearchItem, 0, len(payload.Data))
	for _, entry := range payload.Data {
		obj := mapFromAny(entry["object"])
		if len(obj) == 0 {
			obj = entry
		}
		item := SearchItem{
			Title:   firstString(obj, "title", "name"),
			Excerpt: firstString(obj, "excerpt", "description"),
			URL:     firstString(obj, "url", "link"),
			Type:    firstString(entry, "type"),
		}
		if item.Title == "" {
			item.Title = firstString(entry, "highlight", "title")
		}
		if id := int64FromAny(obj["id"]); item.URL == "" && id != 0 {
			item.URL = questionURL(id, "")
		}
		if item.Title != "" || item.URL != "" {
			items = append(items, item)
		}
	}
	return items, nil
}

func (c *Client) Question(ctx context.Context, questionID int64) (Question, error) {
	if questionID <= 0 {
		return Question{}, errors.New("question_id must be positive")
	}
	var payload struct {
		ID            int64  `json:"id"`
		Title         string `json:"title"`
		Detail        string `json:"detail"`
		Excerpt       string `json:"excerpt"`
		AnswerCount   int    `json:"answer_count"`
		FollowerCount int    `json:"follower_count"`
		URL           string `json:"url"`
	}
	path := fmt.Sprintf("/questions/%d", questionID)
	if err := c.session.FetchJSON(ctx, path, map[string]string{
		"include": "detail,answer_count,follower_count",
	}, &payload); err != nil {
		return Question{}, err
	}
	detail := strings.TrimSpace(payload.Detail)
	if detail == "" {
		detail = strings.TrimSpace(payload.Excerpt)
	}
	return Question{
		ID:          payload.ID,
		Title:       strings.TrimSpace(payload.Title),
		Detail:      detail,
		URL:         questionURL(questionID, payload.URL),
		AnswerCount: payload.AnswerCount,
		Follower:    payload.FollowerCount,
	}, nil
}

func (c *Client) Answers(ctx context.Context, questionID int64, limit int) ([]Answer, error) {
	if questionID <= 0 {
		return nil, errors.New("question_id must be positive")
	}
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	var payload struct {
		Data []struct {
			ID          int64  `json:"id"`
			Excerpt     string `json:"excerpt"`
			URL         string `json:"url"`
			VoteupCount int    `json:"voteup_count"`
			Author      struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/questions/%d/answers", questionID)
	if err := c.session.FetchJSON(ctx, path, map[string]string{
		"limit":   strconv.Itoa(limit),
		"offset":  "0",
		"sort_by": "default",
		"include": "data[*].excerpt,voteup_count,author.name",
	}, &payload); err != nil {
		return nil, err
	}
	answers := make([]Answer, 0, len(payload.Data))
	for _, raw := range payload.Data {
		answers = append(answers, Answer{
			ID:      raw.ID,
			Author:  strings.TrimSpace(raw.Author.Name),
			Excerpt: strings.TrimSpace(raw.Excerpt),
			URL:     answerURL(questionID, raw.ID, raw.URL),
			Votes:   raw.VoteupCount,
		})
	}
	return answers, nil
}

func (c *Client) PublishAnswer(ctx context.Context, req PublishAnswerRequest) (PublishAnswerResult, error) {
	req.Content = strings.TrimSpace(req.Content)
	if req.QuestionID <= 0 {
		return PublishAnswerResult{}, errors.New("question_id must be positive")
	}
	if req.Content == "" {
		return PublishAnswerResult{}, errors.New("content is required")
	}
	result := PublishAnswerResult{
		DryRun:     req.DryRun,
		QuestionID: req.QuestionID,
		URL:        questionURL(req.QuestionID, ""),
		Content:    req.Content,
	}
	if req.DryRun {
		result.Message = "dry run only; pass dry_run=false after logging in with zhihu_open_login"
		return result, nil
	}

	var payload struct {
		ID  any    `json:"id"`
		URL string `json:"url"`
	}
	path := fmt.Sprintf("https://www.zhihu.com/api/v4/questions/%d/answers", req.QuestionID)
	if err := c.session.PostJSON(ctx, path, map[string]any{
		"content":             plainTextToZhihuHTML(req.Content),
		"reshipment_settings": "allowed",
		"comment_permission":  "all",
		"reward_setting":      map[string]any{"can_reward": false},
	}, &payload); err != nil {
		return PublishAnswerResult{}, err
	}
	result.AnswerID = int64FromAny(payload.ID)
	result.URL = answerURL(req.QuestionID, result.AnswerID, payload.URL)
	result.Message = "answer published"
	return result, nil
}

func (c *Client) PublishArticle(ctx context.Context, req PublishArticleRequest) (PublishArticleResult, error) {
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	if req.Title == "" {
		return PublishArticleResult{}, errors.New("title is required")
	}
	if req.Content == "" {
		return PublishArticleResult{}, errors.New("content is required")
	}
	result := PublishArticleResult{
		DryRun:  req.DryRun,
		Title:   req.Title,
		Content: req.Content,
	}
	if req.DryRun {
		result.Message = "dry run only; pass dry_run=false after logging in with zhihu_open_login"
		return result, nil
	}

	var draft struct {
		ID  any    `json:"id"`
		URL string `json:"url"`
	}
	if err := c.session.RequestJSON(ctx, "POST", "https://zhuanlan.zhihu.com/api/articles/drafts", map[string]any{}, &draft); err != nil {
		return PublishArticleResult{}, fmt.Errorf("create zhihu article draft: %w", err)
	}
	draftID := int64FromAny(draft.ID)
	if draftID <= 0 {
		return PublishArticleResult{}, errors.New("create zhihu article draft: response did not include draft id")
	}

	contentHTML := plainTextToZhihuHTML(req.Content)
	draftURL := fmt.Sprintf("https://zhuanlan.zhihu.com/api/articles/%d/draft", draftID)
	if err := c.session.RequestJSON(ctx, "PATCH", draftURL, map[string]any{
		"title":             req.Title,
		"content":           contentHTML,
		"table_of_contents": false,
	}, &draft); err != nil {
		return PublishArticleResult{}, fmt.Errorf("update zhihu article draft: %w", err)
	}

	var publishPayload struct {
		Data struct {
			Result string `json:"result"`
		} `json:"data"`
	}
	publishBody := map[string]any{
		"action": "publish",
		"data": map[string]any{
			"type":                         "article",
			"article_id":                   draftID,
			"title":                        req.Title,
			"content":                      contentHTML,
			"column":                       nil,
			"comment_permission":           "all",
			"commercial_report_info":       map[string]any{"commercial_types": []any{}},
			"commercial_zhitask_bind_info": nil,
			"content_source":               map[string]any{"method": 0},
		},
	}
	if err := c.session.RequestJSON(ctx, "POST", "https://www.zhihu.com/api/v4/content/publish", publishBody, &publishPayload); err != nil {
		return PublishArticleResult{}, fmt.Errorf("publish zhihu article: %w", err)
	}
	if strings.TrimSpace(publishPayload.Data.Result) != "" {
		_ = json.Unmarshal([]byte(publishPayload.Data.Result), &draft)
	}

	result.ArticleID = int64FromAny(draft.ID)
	if result.ArticleID <= 0 {
		result.ArticleID = draftID
	}
	result.URL = articleURL(result.ArticleID, draft.URL)
	result.Message = "article published"
	return result, nil
}

func (c *Client) UpdateAnswer(ctx context.Context, req UpdateAnswerRequest) (UpdateAnswerResult, error) {
	req.Content = strings.TrimSpace(req.Content)
	req.ContentHTML = strings.TrimSpace(req.ContentHTML)
	if req.QuestionID <= 0 {
		return UpdateAnswerResult{}, errors.New("question_id must be positive")
	}
	if req.AnswerID <= 0 {
		return UpdateAnswerResult{}, errors.New("answer_id must be positive")
	}
	if req.Content == "" && req.ContentHTML == "" {
		return UpdateAnswerResult{}, errors.New("content or content_html is required")
	}
	result := UpdateAnswerResult{
		DryRun:     req.DryRun,
		QuestionID: req.QuestionID,
		AnswerID:   req.AnswerID,
		URL:        answerURL(req.QuestionID, req.AnswerID, ""),
		Content:    displayContent(req.Content, req.ContentHTML),
	}
	if req.DryRun {
		result.Message = "dry run only; pass dry_run=false after logging in with zhihu_open_login"
		return result, nil
	}

	contentHTML := contentHTML(req.Content, req.ContentHTML)
	publishBody := map[string]any{
		"action": "update",
		"data": map[string]any{
			"type":                "answer",
			"question_id":         req.QuestionID,
			"answer_id":           req.AnswerID,
			"content":             contentHTML,
			"reshipment_settings": "allowed",
			"comment_permission":  "all",
			"reward_setting":      map[string]any{"can_reward": false},
			"is_copyable":         true,
			"is_report":           false,
		},
	}
	var payload contentPublishResponse
	if err := c.session.RequestJSON(ctx, "POST", "https://www.zhihu.com/api/v4/content/publish", publishBody, &payload); err != nil {
		return UpdateAnswerResult{}, fmt.Errorf("update zhihu answer: %w", err)
	}
	result.Message = "answer updated"
	return result, nil
}

func (c *Client) UpdateArticle(ctx context.Context, req UpdateArticleRequest) (UpdateArticleResult, error) {
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	req.ContentHTML = strings.TrimSpace(req.ContentHTML)
	if req.ArticleID <= 0 {
		return UpdateArticleResult{}, errors.New("article_id must be positive")
	}
	if req.Title == "" {
		return UpdateArticleResult{}, errors.New("title is required")
	}
	if req.Content == "" && req.ContentHTML == "" {
		return UpdateArticleResult{}, errors.New("content or content_html is required")
	}
	result := UpdateArticleResult{
		DryRun:    req.DryRun,
		ArticleID: req.ArticleID,
		URL:       articleURL(req.ArticleID, ""),
		Title:     req.Title,
		Content:   displayContent(req.Content, req.ContentHTML),
	}
	if req.DryRun {
		result.Message = "dry run only; pass dry_run=false after logging in with zhihu_open_login"
		return result, nil
	}

	contentHTML := contentHTML(req.Content, req.ContentHTML)
	publishBody := map[string]any{
		"action": "update",
		"data": map[string]any{
			"type":                         "article",
			"article_id":                   req.ArticleID,
			"title":                        req.Title,
			"content":                      contentHTML,
			"column":                       nil,
			"comment_permission":           "all",
			"commercial_report_info":       map[string]any{"commercial_types": []any{}},
			"commercial_zhitask_bind_info": nil,
			"content_source":               map[string]any{"method": 0},
		},
	}
	var payload contentPublishResponse
	if err := c.session.RequestJSON(ctx, "POST", "https://www.zhihu.com/api/v4/content/publish", publishBody, &payload); err != nil {
		return UpdateArticleResult{}, fmt.Errorf("update zhihu article: %w", err)
	}
	if strings.TrimSpace(payload.Data.Result) != "" {
		var published struct {
			ID  any    `json:"id"`
			URL string `json:"url"`
		}
		if err := json.Unmarshal([]byte(payload.Data.Result), &published); err == nil {
			if id := int64FromAny(published.ID); id > 0 {
				result.ArticleID = id
			}
			result.URL = articleURL(result.ArticleID, published.URL)
		}
	}
	result.Message = "article updated"
	return result, nil
}

type contentPublishResponse struct {
	Data struct {
		Result string `json:"result"`
	} `json:"data"`
}

var questionURLPattern = regexp.MustCompile(`/questions?/(\d+)`)

func questionIDFromURL(rawURL string) int64 {
	match := questionURLPattern.FindStringSubmatch(rawURL)
	if len(match) != 2 {
		return 0
	}
	id, _ := strconv.ParseInt(match[1], 10, 64)
	return id
}

func questionURL(id int64, fallback string) string {
	if id > 0 {
		return fmt.Sprintf("https://www.zhihu.com/question/%d", id)
	}
	return fallback
}

func answerURL(questionID, answerID int64, fallback string) string {
	if questionID > 0 && answerID > 0 {
		return fmt.Sprintf("https://www.zhihu.com/question/%d/answer/%d", questionID, answerID)
	}
	return fallback
}

func articleURL(articleID int64, fallback string) string {
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	if articleID > 0 {
		return fmt.Sprintf("https://zhuanlan.zhihu.com/p/%d", articleID)
	}
	return ""
}

func plainTextToZhihuHTML(content string) string {
	lines := strings.Split(content, "\n")
	var paragraphs []string
	var current []string
	flush := func() {
		text := strings.TrimSpace(strings.Join(current, "\n"))
		if text != "" {
			paragraphs = append(paragraphs, "<p>"+htmlEscapeWithBreaks(text)+"</p>")
		}
		current = nil
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return strings.Join(paragraphs, "")
}

func htmlEscapeWithBreaks(content string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
		"\n", "<br>",
	)
	return replacer.Replace(content)
}

func contentHTML(content, html string) string {
	if strings.TrimSpace(html) != "" {
		return strings.TrimSpace(html)
	}
	return plainTextToZhihuHTML(content)
}

func displayContent(content, html string) string {
	if strings.TrimSpace(content) != "" {
		return strings.TrimSpace(content)
	}
	return strings.TrimSpace(html)
}

func mapFromAny(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func int64FromAny(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		id, _ := strconv.ParseInt(v, 10, 64)
		return id
	default:
		return 0
	}
}

func defaultProfileDir() string {
	return filepath.Join(".", ".zhihu-profile")
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func encodeURL(baseURL, path string, params map[string]string) (string, error) {
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + path)
	if err != nil {
		return "", err
	}
	q := endpoint.Query()
	for key, value := range params {
		q.Set(key, value)
	}
	endpoint.RawQuery = q.Encode()
	return endpoint.String(), nil
}
