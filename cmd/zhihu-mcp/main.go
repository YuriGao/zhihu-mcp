package main

import (
	"context"
	"fmt"
	"os"

	"github.com/YuriGao/zhihu-mcp/internal/mcp"
	"github.com/YuriGao/zhihu-mcp/internal/zhihu"
)

func main() {
	server := mcp.NewServer(zhihu.NewClient(zhihu.WithCookie(os.Getenv("ZHIHU_COOKIE"))))
	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "zhihu-mcp: %v\n", err)
		os.Exit(1)
	}
}
