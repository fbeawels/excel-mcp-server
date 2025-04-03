package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fbeawels/excel-mcp-server/internal/server"
)

var (
  version = "dev"
)

func main() {
	// Define command-line flags
	transportPtr := flag.String("transport", "", "Transport type (stdio or sse)")
	hostPtr := flag.String("host", "", "Host address to listen on for SSE transport (default: all interfaces)")
	portPtr := flag.String("port", "", "Port for SSE transport (default: 8080)")
	flag.Parse()

	// Set environment variables based on command-line arguments if provided
	if *transportPtr != "" {
		os.Setenv("EXCEL_MCP_TRANSPORT", *transportPtr)
	}

	if *hostPtr != "" {
		os.Setenv("EXCEL_MCP_HOST", *hostPtr)
	}

	if *portPtr != "" {
		os.Setenv("EXCEL_MCP_PORT", *portPtr)
	}

	// Create and start the server
	s := server.New(version)
	err := s.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start the server: %v\n", err)
		os.Exit(1)
	}
}
