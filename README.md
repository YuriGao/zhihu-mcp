# zhihu-mcp

A Go MCP server for Zhihu using Playwright persistent browser state over stdio.

## Tools

- `zhihu_open_login`: open a visible Playwright browser for manual Zhihu login.
- `zhihu_login_status`: check whether the persistent profile appears logged in.
- `zhihu_hot_list`: fetch Zhihu hot list items.
- `zhihu_search`: search Zhihu by keyword.
- `zhihu_question`: fetch question metadata.
- `zhihu_answers`: fetch answers for a question.
- `zhihu_publish_answer`: publish an answer with the persistent Playwright profile.
- `zhihu_publish_article`: publish a column article with the persistent Playwright profile.

The server stores login state in a dedicated Playwright profile directory. It does not read your normal Chrome profile and does not bypass captcha, rate limits, or other Zhihu safety checks.

## Install

Install the Go dependencies and Playwright Chromium browser:

```powershell
go mod download
go run github.com/mxschmitt/playwright-go/cmd/playwright install chromium
```

## Build

```powershell
go build ./cmd/zhihu-mcp
```

## Run

```powershell
go run ./cmd/zhihu-mcp
```

Optional environment variables:

```powershell
$env:ZHIHU_PROFILE_DIR = ".zhihu-profile"
$env:ZHIHU_HEADLESS = "true"
go run ./cmd/zhihu-mcp
```

`ZHIHU_PROFILE_DIR` defaults to `./.zhihu-profile`. Keep this directory private; it contains browser login state.

## Login Flow

1. Start the MCP server.
2. Call `zhihu_open_login`.
3. Complete Zhihu login in the visible browser window.
4. Call `zhihu_login_status` to verify the persistent profile is logged in.
5. Use the read or publish tools. Future runs reuse the saved profile.

## Publishing Answers

`zhihu_publish_answer` defaults to `dry_run: true`, so the first call only previews the payload. To publish, call it with:

```json
{
  "question_id": 123,
  "content": "Your answer content",
  "dry_run": false
}
```

Publishing requires that the Playwright profile is logged in. The server uses normal browser cookies and `_xsrf` from that profile.

## Publishing Articles

`zhihu_publish_article` also defaults to `dry_run: true`. To publish a column article, call it with:

```json
{
  "title": "Your article title",
  "content": "Plain-text article content",
  "content_html": "<h2>Optional rich section</h2><p>Zhihu-compatible HTML.</p>",
  "dry_run": false
}
```

The server converts plain text paragraphs into Zhihu-compatible HTML unless `content_html` is provided. Because Zhihu article publishing may leave content as a draft, verify the returned URL in the browser after publishing.

## Updating Published Content

Published-content update tools are intentionally not exposed yet. Zhihu's editor/publish flow can create drafts that look successful at the API layer without changing the visible published content, and the article update path can make an existing article unavailable.

Until the update flow is verified end to end, edit published answers and articles in the Zhihu UI, or publish a replacement article with `zhihu_publish_article`.

## MCP Configuration

For an installed binary:

```json
{
  "mcpServers": {
    "zhihu": {
      "command": "zhihu-mcp"
    }
  }
}
```

During local development:

```json
{
  "mcpServers": {
    "zhihu": {
      "command": "go",
      "args": ["run", "./cmd/zhihu-mcp"],
      "env": {
        "ZHIHU_PROFILE_DIR": ".zhihu-profile",
        "ZHIHU_HEADLESS": "true"
      }
    }
  }
}
```
