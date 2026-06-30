package zhihu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mxschmitt/playwright-go"
)

type PlaywrightSessionConfig struct {
	BaseURL    string
	ProfileDir string
	Headless   bool
}

type PlaywrightSession struct {
	baseURL    string
	profileDir string

	mu       sync.Mutex
	headless bool
	pw       *playwright.Playwright
	context  playwright.BrowserContext
	page     playwright.Page
}

func NewPlaywrightSession(cfg PlaywrightSessionConfig) *PlaywrightSession {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.ProfileDir == "" {
		cfg.ProfileDir = defaultProfileDir()
	}
	return &PlaywrightSession{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		profileDir: cfg.ProfileDir,
		headless:   cfg.Headless,
	}
}

func (s *PlaywrightSession) FetchJSON(ctx context.Context, path string, params map[string]string, target any) error {
	_ = ctx
	page, err := s.ensurePage(s.headless)
	if err != nil {
		return err
	}
	apiURL, err := encodeURL(s.baseURL, path, params)
	if err != nil {
		return err
	}
	if _, err := page.Goto(apiURL); err != nil {
		return err
	}
	body, err := page.Locator("body").TextContent()
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(body), target); err != nil {
		return fmt.Errorf("decode zhihu response from browser: %w", err)
	}
	return nil
}

func (s *PlaywrightSession) PostJSON(ctx context.Context, path string, body any, target any) error {
	return s.RequestJSON(ctx, "POST", path, body, target)
}

func (s *PlaywrightSession) RequestJSON(ctx context.Context, method string, path string, body any, target any) error {
	_ = ctx
	page, err := s.ensurePage(s.headless)
	if err != nil {
		return err
	}
	navigationURL := "https://www.zhihu.com"
	questionID := questionIDFromURL(path)
	if questionID > 0 {
		navigationURL = questionURL(questionID, "")
	} else if strings.HasPrefix(path, "https://zhuanlan.zhihu.com") {
		navigationURL = "https://zhuanlan.zhihu.com/write"
	}
	if _, err := page.Goto(navigationURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	}); err != nil {
		return err
	}
	_ = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateDomcontentloaded,
		Timeout: playwright.Float(5000),
	})
	apiURL, err := s.requestURL(path)
	if err != nil {
		return err
	}
	arg := map[string]any{
		"method": strings.ToUpper(method),
		"url":    apiURL,
		"body":   body,
	}
	value, err := evaluateBrowserJSONRequest(page, arg)
	if err != nil {
		return err
	}
	result, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("unexpected browser fetch result: %T", value)
	}
	text, _ := result["text"].(string)
	if okValue, _ := result["ok"].(bool); !okValue {
		status, _ := result["status"].(float64)
		statusText, _ := result["statusText"].(string)
		return fmt.Errorf("zhihu request failed: %.0f %s: %s", status, statusText, strings.TrimSpace(text))
	}
	if target == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(text), target); err != nil {
		return fmt.Errorf("decode zhihu response from browser: %w", err)
	}
	return nil
}

func (s *PlaywrightSession) requestURL(path string) (string, error) {
	if strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
		return path, nil
	}
	return encodeURL(s.baseURL, path, nil)
}

func evaluateBrowserJSONRequest(page playwright.Page, arg map[string]any) (any, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		value, err := page.Evaluate(`async ({ method, url, body }) => {
			const xsrf = document.cookie.split('; ')
				.find((part) => part.startsWith('_xsrf='))
				?.split('=')
				.slice(1)
				.join('=') || '';
			const response = await fetch(url, {
				method,
				credentials: 'include',
				headers: {
					'accept': 'application/json, text/plain, */*',
					'content-type': 'application/json',
					'x-xsrftoken': decodeURIComponent(xsrf)
				},
				body: JSON.stringify(body || {})
			});
			const text = await response.text();
			return { ok: response.ok, status: response.status, statusText: response.statusText, text };
		}`, arg)
		if err == nil {
			return value, nil
		}
		lastErr = err
		if !isNavigationContextError(err) {
			return nil, err
		}
		_ = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
			State:   playwright.LoadStateDomcontentloaded,
			Timeout: playwright.Float(3000),
		})
	}
	return nil, lastErr
}

func isNavigationContextError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "Execution context was destroyed") ||
		strings.Contains(message, "Cannot find context") ||
		strings.Contains(message, "Most likely because of a navigation")
}

func (s *PlaywrightSession) OpenLogin(ctx context.Context) (LoginResult, error) {
	_ = ctx
	page, err := s.ensurePage(false)
	if err != nil {
		return LoginResult{}, err
	}
	loginURL := "https://www.zhihu.com/signin"
	if _, err := page.Goto(loginURL); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		LoginURL:   loginURL,
		ProfileDir: s.profileDir,
		Message:    "login page opened in a persistent Playwright profile; complete login in the browser window",
	}, nil
}

func (s *PlaywrightSession) LoginStatus(ctx context.Context) (LoginStatus, error) {
	_ = ctx
	page, err := s.ensurePage(s.headless)
	if err != nil {
		return LoginStatus{}, err
	}
	if _, err := page.Goto("https://www.zhihu.com"); err != nil {
		return LoginStatus{}, err
	}
	value, err := page.Evaluate(`() => {
		const cookieLoggedIn = document.cookie.includes('z_c0=');
		const hasLoginLink = !!document.querySelector('a[href*="signin"], button.SignFlow-submitButton');
		return {
			loggedIn: cookieLoggedIn || !hasLoginLink,
			url: location.href
		};
	}`)
	if err != nil {
		return LoginStatus{}, err
	}
	result, _ := value.(map[string]any)
	loggedIn, _ := result["loggedIn"].(bool)
	currentURL, _ := result["url"].(string)
	message := "not logged in; call zhihu_open_login and finish login in the browser window"
	if loggedIn {
		message = "logged in with persistent Playwright profile"
	}
	return LoginStatus{
		LoggedIn:   loggedIn,
		ProfileDir: s.profileDir,
		URL:        currentURL,
		Message:    message,
	}, nil
}

func (s *PlaywrightSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

func (s *PlaywrightSession) ensurePage(headless bool) (playwright.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.context != nil && s.page != nil && s.headless == headless {
		return s.page, nil
	}
	if s.context != nil || s.pw != nil {
		if err := s.closeLocked(); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(s.profileDir, 0o700); err != nil {
		return nil, err
	}
	pw, err := playwright.Run()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "please install") {
			return nil, fmt.Errorf("%w; run: go run github.com/mxschmitt/playwright-go/cmd/playwright install chromium", err)
		}
		return nil, err
	}
	context, err := pw.Chromium.LaunchPersistentContext(s.profileDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(headless),
		Locale:   playwright.String("zh-CN"),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
		},
	})
	if err != nil {
		_ = pw.Stop()
		return nil, err
	}
	page, err := context.NewPage()
	if err != nil {
		_ = context.Close()
		_ = pw.Stop()
		return nil, err
	}
	s.pw = pw
	s.context = context
	s.page = page
	s.headless = headless
	return page, nil
}

func (s *PlaywrightSession) closeLocked() error {
	var errs []string
	if s.page != nil {
		if err := s.page.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if s.context != nil {
		if err := s.context.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if s.pw != nil {
		if err := s.pw.Stop(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	s.page = nil
	s.context = nil
	s.pw = nil
	if len(errs) > 0 {
		return fmt.Errorf("close playwright session: %s", strings.Join(errs, "; "))
	}
	return nil
}

func absProfileDir(profileDir string) string {
	if filepath.IsAbs(profileDir) {
		return profileDir
	}
	abs, err := filepath.Abs(profileDir)
	if err != nil {
		return profileDir
	}
	return abs
}
