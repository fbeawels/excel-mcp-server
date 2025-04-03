package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"

	"github.com/mark3labs/mcp-go/server"
	"github.com/fbeawels/excel-mcp-server/internal/tools"
)

type TransportType string

const (
	TransportStdio TransportType = "stdio"
	TransportSSE   TransportType = "sse"
)

type ExcelServer struct {
	server        *server.MCPServer
	transportType TransportType
	host          string
	port          int
}

func New(version string) *ExcelServer {
	s := &ExcelServer{
		transportType: TransportStdio, // Default to stdio
		host:          "",             // Default to all interfaces
		port:          8000,           // Default port for SSE
	}

	// Check if SSE transport is requested via environment variable
	if transportEnv := os.Getenv("EXCEL_MCP_TRANSPORT"); transportEnv == "sse" {
		s.transportType = TransportSSE
		
		// Check if a custom host is specified
		if hostEnv := os.Getenv("EXCEL_MCP_HOST"); hostEnv != "" {
			s.host = hostEnv
		}

		// Check if a custom port is specified
		if portEnv := os.Getenv("EXCEL_MCP_PORT"); portEnv != "" {
			if port, err := strconv.Atoi(portEnv); err == nil {
				s.port = port
			} else {
				log.Printf("Warning: Invalid port specified in EXCEL_MCP_PORT: %s. Using default port %d", portEnv, s.port)
			}
		}
	}

	s.server = server.NewMCPServer(
		"excel-mcp-server",
		version,
	)
	tools.AddReadSheetNamesTool(s.server)
	tools.AddReadSheetDataTool(s.server)
	tools.AddReadSheetFormulaTool(s.server)
	if runtime.GOOS == "windows" {
		tools.AddReadSheetImageTool(s.server)
	}
	tools.AddWriteSheetDataTool(s.server)
	tools.AddWriteSheetFormulaTool(s.server)
	return s
}

func (s *ExcelServer) Start() error {
	switch s.transportType {
	case TransportStdio:
		return server.ServeStdio(s.server)
	case TransportSSE:
		return s.serveSSE()
	default:
		return fmt.Errorf("unsupported transport type: %s", s.transportType)
	}
}

// serveSSE starts an HTTP server that serves the MCP server over SSE
func (s *ExcelServer) serveSSE() error {
	// Handle both the simple /sse endpoint and the langflow-compatible /api/v1/mcp/sse endpoint
	sseHandler := func(w http.ResponseWriter, r *http.Request) {
		// Set headers for SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Create a channel for SSE events
		ch := make(chan []byte)

		// Start the MCP server with the SSE channel
		go func() {
			// This is a simplified implementation. In a real implementation,
			// we would need to adapt the MCP server to use the SSE channel for output
			// and to read input from HTTP requests.
			// For now, we'll just log that SSE is not fully implemented
			log.Println("SSE transport is enabled but the implementation is incomplete.")
			log.Println("A full implementation would require modifications to the MCP library.")
			
			// Keep the connection alive
			for {
				select {
				case <-r.Context().Done():
					close(ch)
					return
				}
			}
		}()

		// Send events to the client
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		for data := range ch {
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}

	// Register the SSE handler for both endpoints
	http.HandleFunc("/sse", sseHandler)
	http.HandleFunc("/api/v1/mcp/sse", sseHandler)

	// Add an endpoint to send commands to the MCP server
	commandHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read the command from the request body
		// In a real implementation, we would pass this to the MCP server
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"received"}`)) 
	}

	// Register the command handler for both endpoints
	http.HandleFunc("/command", commandHandler)
	http.HandleFunc("/api/v1/mcp/command", commandHandler)

	// Add a simple status endpoint
	statusHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"running","transport":"sse"}`)) 
	}

	// Register the status handler for both endpoints
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/api/v1/mcp/status", statusHandler)

	// Start the HTTP server
	address := fmt.Sprintf("%s:%d", s.host, s.port)
	log.Printf("Starting Excel MCP Server with SSE transport on %s", address)
	
	// Determine the host to display in URLs
	displayHost := "localhost"
	if s.host == "0.0.0.0" || s.host == "" {
		// When binding to all interfaces, still use localhost for display
		displayHost = "localhost"
	} else {
		displayHost = s.host
	}
	
	log.Printf("SSE endpoint available at http://%s:%d/sse", displayHost, s.port)
	log.Printf("Langflow-compatible SSE endpoint available at http://%s:%d/api/v1/mcp/sse", displayHost, s.port)
	log.Printf("Command endpoint available at http://%s:%d/command", displayHost, s.port)
	log.Printf("Status endpoint available at http://%s:%d/status", displayHost, s.port)
	return http.ListenAndServe(address, nil)
}
