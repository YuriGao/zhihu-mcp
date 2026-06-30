# zhihu-mcp

A small Go MCP server for public Zhihu data over stdio.

## Tools

- `zhihu_hot_list`: fetch Zhihu hot list items.
- `zhihu_search`: search Zhihu by keyword.
- `zhihu_question`: fetch question metadata.
- `zhihu_answers`: fetch answers for a question.
- `zhihu_publish_answer`: publish an answer to a question with your own Zhihu login cookie.

`zhihu_hot_list` works against a public Zhihu endpoint. Some deeper APIs, especially search and answers, may require a logged-in Zhihu session or may be rate-limited by Zhihu. If needed, provide a browser cookie through `ZHIHU_COOKIE`.

## Build

```powershell
go build ./cmd/zhihu-mcp
```

## Run

```powershell
go run ./cmd/zhihu-mcp
```

Optional cookie:

```powershell
$env:ZHIHU_COOKIE = "_xsrf=...; z_c0=..."
go run ./cmd/zhihu-mcp
```

## Publishing Answers

`zhihu_publish_answer` defaults to `dry_run: true`, so the first call only previews the payload. To publish, call it with:

```json
{
  "question_id": 123,
  "content": "Your answer content",
  "dry_run": false
}
```

Publishing requires `ZHIHU_COOKIE` from your own logged-in Zhihu session. The server sends the cookie and `_xsrf` token normally; it does not bypass captcha, rate limits, or other Zhihu safety checks.

MCP clients should configure the command as:

```json
{
  "mcpServers": {
    "zhihu": {
      "command": "zhihu-mcp"
    }
  }
}
```

During local development, use:

```json
{
  "mcpServers": {
    "zhihu": {
      "command": "go",
      "args": ["run", "./cmd/zhihu-mcp"]
    }
  }
}
```
