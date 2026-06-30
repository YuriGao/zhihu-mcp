package main

import (
	"context"
	"fmt"
	"os"

	"github.com/YuriGao/zhihu-mcp/internal/mcp"
	"github.com/YuriGao/zhihu-mcp/internal/zhihu"
)

func main() {
	client := zhihu.NewClient()
	defer client.Close()

	server := mcp.NewServer(client)
	if err := server.Serve(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "zhihu-mcp: %v\n", err)
		os.Exit(1)
	}
}
