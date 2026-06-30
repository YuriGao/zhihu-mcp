package zhihu

import (
	"context"
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
		ID  int64  `json:"id"`
		URL string `json:"url"`
	}
	path := fmt.Sprintf("/questions/%d/answers", req.QuestionID)
	if err := c.session.PostJSON(ctx, path, map[string]any{
		"content": req.Content,
	}, &payload); err != nil {
		return PublishAnswerResult{}, err
	}
	result.AnswerID = payload.ID
	result.URL = answerURL(req.QuestionID, payload.ID, payload.URL)
	result.Message = "answer published"
	return result, nil
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
