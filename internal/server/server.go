package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/fbeawels/excel-mcp-server/internal/tools"
	"github.com/xuri/excelize/v2"
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

// SSEClient represents a connected SSE client
type SSEClient struct {
	ID       string
	Messages chan []byte
}

// SSEManager manages all SSE client connections
type SSEManager struct {
	clients    map[string]*SSEClient
	register   chan *SSEClient
	unregister chan *SSEClient
	broadcast  chan []byte
	mutex      sync.Mutex
}

// NewSSEManager creates a new SSE manager
func NewSSEManager() *SSEManager {
	return &SSEManager{
		clients:    make(map[string]*SSEClient),
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		broadcast:  make(chan []byte),
	}
}

// Run starts the SSE manager
func (m *SSEManager) Run(ctx context.Context) {
	// Start a heartbeat ticker
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-m.register:
			m.mutex.Lock()
			m.clients[client.ID] = client
			log.Printf("Client connected: %s (total: %d)", client.ID, len(m.clients))
			m.mutex.Unlock()

		case client := <-m.unregister:
			m.mutex.Lock()
			if _, ok := m.clients[client.ID]; ok {
				delete(m.clients, client.ID)
				close(client.Messages)
				log.Printf("Client disconnected: %s (remaining: %d)", client.ID, len(m.clients))
			}
			m.mutex.Unlock()

		case message := <-m.broadcast:
			m.mutex.Lock()
			for _, client := range m.clients {
				select {
				case client.Messages <- message:
					// Message sent successfully
				default:
					// Client's message buffer is full, unregister it
					close(client.Messages)
					delete(m.clients, client.ID)
				}
			}
			m.mutex.Unlock()

		case <-ticker.C:
			// Send heartbeat to all clients in JSON-RPC format
			heartbeat := map[string]interface{}{
				"jsonrpc":   "2.0",
				"method":    "heartbeat",
				"params": map[string]interface{}{
					"timestamp": time.Now().Format(time.RFC3339),
				},
			}
			heartbeatJSON, _ := json.Marshal(heartbeat)
			m.mutex.Lock()
			for _, client := range m.clients {
				select {
				case client.Messages <- heartbeatJSON:
					// Heartbeat sent successfully
				default:
					// Client's message buffer is full, unregister it
					close(client.Messages)
					delete(m.clients, client.ID)
				}
			}
			m.mutex.Unlock()

		case <-ctx.Done():
			// Context canceled, close all client connections
			m.mutex.Lock()
			for _, client := range m.clients {
				close(client.Messages)
			}
			m.clients = make(map[string]*SSEClient)
			m.mutex.Unlock()
			return
		}
	}
}

// serveSSE starts an HTTP server that serves the MCP server over SSE
func (s *ExcelServer) serveSSE() error {
	// Create a new SSE manager
	manager := NewSSEManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the SSE manager
	go manager.Run(ctx)

	// Add a middleware to handle CORS
	corsMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Set CORS headers for all responses
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, X-Requested-With")
			w.Header().Set("Access-Control-Max-Age", "3600")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			// Call the next handler
			next(w, r)
		}
	}

	// Handle both the simple /sse endpoint and the langflow-compatible /api/v1/mcp/sse endpoint
	sseHandler := func(w http.ResponseWriter, r *http.Request) {
		// Special handling for HEAD requests which Langflow uses to check connectivity
		if r.Method == "HEAD" {
			log.Printf("HEAD request for SSE from %s", r.RemoteAddr)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			return
		}

		// Log the request
		log.Printf("SSE connection request from %s", r.RemoteAddr)

		// Set headers for SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		
		// Check if the client supports flushing
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Create a new client
		clientID := fmt.Sprintf("%d", time.Now().UnixNano())
		client := &SSEClient{
			ID:       clientID,
			Messages: make(chan []byte, 100), // Buffer up to 100 messages
		}

		// Register the client
		manager.register <- client

		// Send an immediate connection message in JSON-RPC format
		jsonrpcMsg := map[string]interface{}{
			"jsonrpc": "2.0",
			"method": "connection",
			"params": map[string]interface{}{
				"message": "Connected to Excel MCP Server",
				"id": clientID,
			},
		}
		jsonrpcData, _ := json.Marshal(jsonrpcMsg)
		fmt.Fprintf(w, "data: %s\n\n", jsonrpcData)
		flusher.Flush()

		// Ensure client is unregistered when the connection is closed
		defer func() {
			manager.unregister <- client
		}()

		// Set up a done channel to notify if the client disconnects
		done := make(chan bool)
		
		// Monitor for client disconnection
		go func() {
			<-r.Context().Done()
			done <- true
		}()

		// Start a heartbeat ticker
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		// Send events to the client
		for {
			select {
			case message, ok := <-client.Messages:
				if !ok {
					// Channel was closed
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", message)
				flusher.Flush()
			case <-ticker.C:
				// Send a heartbeat directly from this handler in JSON-RPC format
				heartbeatMsg := map[string]interface{}{
					"jsonrpc": "2.0",
					"method": "heartbeat",
					"params": map[string]interface{}{
						"timestamp": time.Now().Format(time.RFC3339),
					},
				}
				heartbeatData, _ := json.Marshal(heartbeatMsg)
				fmt.Fprintf(w, "data: %s\n\n", heartbeatData)
				flusher.Flush()
			case <-done:
				// Client disconnected
				return
			}
		}
	}

	// Register the SSE handler for both endpoints with CORS middleware
	http.HandleFunc("/sse", corsMiddleware(sseHandler))
	http.HandleFunc("/api/v1/mcp/sse", corsMiddleware(sseHandler))

	// Add an endpoint to send commands to the MCP server
	commandHandler := func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Content-Type", "application/json")
		
		if r.Method != http.MethodPost {
			http.Error(w, "{\"error\":\"Method not allowed\"}", http.StatusMethodNotAllowed)
			return
		}

		// Read the command from the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("{\"error\":\"Failed to read request body: %s\"}", err), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Parse the JSON-RPC request
		var jsonrpcRequest struct {
			Jsonrpc string          `json:"jsonrpc"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params,omitempty"`
			ID      interface{}     `json:"id,omitempty"`
		}

		if err := json.Unmarshal(body, &jsonrpcRequest); err != nil {
			// Try parsing as a simple command if not JSON-RPC format
			var simpleCommand struct {
				Command string          `json:"command"`
				Params  json.RawMessage `json:"params,omitempty"`
			}
			
			if err := json.Unmarshal(body, &simpleCommand); err != nil {
				http.Error(w, fmt.Sprintf("{\"error\":\"Failed to parse command: %s\"}", err), http.StatusBadRequest)
				return
			}
			
			// Convert simple command to JSON-RPC format
			jsonrpcRequest.Jsonrpc = "2.0"
			jsonrpcRequest.Method = simpleCommand.Command
			jsonrpcRequest.Params = simpleCommand.Params
			jsonrpcRequest.ID = 1 // Default ID
		}

		// Log the received command
		log.Printf("Received JSON-RPC request: method=%s, params=%s, id=%v", 
			jsonrpcRequest.Method, string(jsonrpcRequest.Params), jsonrpcRequest.ID)

		// Process the command
		var responseObj interface{}
		var errorObj *map[string]interface{}

		switch jsonrpcRequest.Method {
		case "list_tools":
			// Return the list of available tools
			tools := []map[string]interface{}{
				{
					"name": "read_sheet_names",
					"description": "Read the names of all sheets in an Excel file",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"fileAbsolutePath": map[string]interface{}{
								"type": "string",
								"description": "Absolute path to the Excel file",
							},
						},
						"required": []string{"fileAbsolutePath"},
					},
				},
				{
					"name": "read_sheet_data",
					"description": "Read data from a specific sheet in an Excel file",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"fileAbsolutePath": map[string]interface{}{
								"type": "string",
								"description": "Absolute path to the Excel file",
							},
							"sheetName": map[string]interface{}{
								"type": "string",
								"description": "Name of the sheet to read",
							},
						},
						"required": []string{"fileAbsolutePath", "sheetName"},
					},
				},
				{
					"name": "write_sheet_data",
					"description": "Write data to a specific sheet in an Excel file",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"fileAbsolutePath": map[string]interface{}{
								"type": "string",
								"description": "Absolute path to the Excel file",
							},
							"sheetName": map[string]interface{}{
								"type": "string",
								"description": "Name of the sheet to write to",
							},
							"data": map[string]interface{}{
								"type": "array",
								"description": "Data to write to the sheet",
							},
						},
						"required": []string{"fileAbsolutePath", "sheetName", "data"},
					},
				},
			}
			
			responseObj = map[string]interface{}{
				"tools": tools,
			}

		case "status":
			// Return the server status
			responseObj = map[string]interface{}{
				"status":    "running",
				"transport": "sse",
				"clients":   len(manager.clients),
				"uptime":    time.Since(time.Now()).String(),
			}

		case "call_tool":
			// Parse the tool call request
			var toolRequest struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			
			if err := json.Unmarshal(jsonrpcRequest.Params, &toolRequest); err != nil {
				errorObj = &map[string]interface{}{
					"code":    -32602,
					"message": "Invalid params",
					"data":    err.Error(),
				}
			} else {
				// Execute the tool
				log.Printf("Executing tool: %s with arguments: %v", toolRequest.Name, toolRequest.Arguments)
				
				// Handle different tools directly
				switch toolRequest.Name {
				case "read_sheet_names":
					// Get the file path
					filePath, ok := toolRequest.Arguments["fileAbsolutePath"].(string)
					if !ok {
						errorObj = &map[string]interface{}{
							"code":    -32602,
							"message": "Invalid params",
							"data":    "fileAbsolutePath must be a string",
						}
						break
					}
					
					// Open the Excel file
					workbook, err := excelize.OpenFile(filePath)
					if err != nil {
						errorObj = &map[string]interface{}{
							"code":    -32603,
							"message": "Internal error",
							"data":    err.Error(),
						}
						break
					}
					defer workbook.Close()
					
					// Get the sheet list
					sheetList := workbook.GetSheetList()
					responseObj = map[string]interface{}{
						"sheets": sheetList,
					}
					
				case "read_sheet_data":
					// Get the file path and sheet name
					filePath, ok := toolRequest.Arguments["fileAbsolutePath"].(string)
					if !ok {
						errorObj = &map[string]interface{}{
							"code":    -32602,
							"message": "Invalid params",
							"data":    "fileAbsolutePath must be a string",
						}
						break
					}
					
					sheetName, ok := toolRequest.Arguments["sheetName"].(string)
					if !ok {
						errorObj = &map[string]interface{}{
							"code":    -32602,
							"message": "Invalid params",
							"data":    "sheetName must be a string",
						}
						break
					}
					
					// Open the Excel file
					workbook, err := excelize.OpenFile(filePath)
					if err != nil {
						errorObj = &map[string]interface{}{
							"code":    -32603,
							"message": "Internal error",
							"data":    err.Error(),
						}
						break
					}
					defer workbook.Close()
					
					// Get all the rows in the sheet
					rows, err := workbook.GetRows(sheetName)
					if err != nil {
						errorObj = &map[string]interface{}{
							"code":    -32603,
							"message": "Internal error",
							"data":    err.Error(),
						}
						break
					}
					
					responseObj = map[string]interface{}{
						"data": rows,
					}
					
				case "write_sheet_data":
					// Get the file path, sheet name, and data
					filePath, ok := toolRequest.Arguments["fileAbsolutePath"].(string)
					if !ok {
						errorObj = &map[string]interface{}{
							"code":    -32602,
							"message": "Invalid params",
							"data":    "fileAbsolutePath must be a string",
						}
						break
					}
					
					sheetName, ok := toolRequest.Arguments["sheetName"].(string)
					if !ok {
						errorObj = &map[string]interface{}{
							"code":    -32602,
							"message": "Invalid params",
							"data":    "sheetName must be a string",
						}
						break
					}
					
					data, ok := toolRequest.Arguments["data"].([]interface{})
					if !ok {
						errorObj = &map[string]interface{}{
							"code":    -32602,
							"message": "Invalid params",
							"data":    "data must be an array",
						}
						break
					}
					
					// Open or create the Excel file
					var workbook *excelize.File
					workbook, err = excelize.OpenFile(filePath)
					if err != nil {
						// Create a new file if it doesn't exist
						workbook = excelize.NewFile()
					}
					defer workbook.Close()
					
					// Check if the sheet exists, create it if not
					sheetIndex, _ := workbook.GetSheetIndex(sheetName)
					if sheetIndex == -1 {
						workbook.NewSheet(sheetName)
					}
					
					// Write the data to the sheet
					for rowIndex, rowData := range data {
						rowArray, ok := rowData.([]interface{})
						if !ok {
							continue
						}
						
						for colIndex, cellData := range rowArray {
							cellValue := fmt.Sprintf("%v", cellData)
							cell, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
							if err != nil {
								continue
							}
							workbook.SetCellValue(sheetName, cell, cellValue)
						}
					}
					
					// Save the workbook
					if err := workbook.SaveAs(filePath); err != nil {
						errorObj = &map[string]interface{}{
							"code":    -32603,
							"message": "Internal error",
							"data":    err.Error(),
						}
						break
					}
					
					responseObj = map[string]interface{}{
						"success": true,
						"message": "Data written successfully",
					}
					
				default:
					errorObj = &map[string]interface{}{
						"code":    -32601,
						"message": "Method not found",
						"data":    fmt.Sprintf("Unknown tool: %s", toolRequest.Name),
					}
				}
			}

		default:
			// Unknown method
			errorObj = &map[string]interface{}{
				"code":    -32601,
				"message": "Method not found",
				"data":    fmt.Sprintf("Unknown method: %s", jsonrpcRequest.Method),
			}
		}

		// Prepare the JSON-RPC response
		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      jsonrpcRequest.ID,
		}
		
		if errorObj != nil {
			response["error"] = *errorObj
		} else {
			response["result"] = responseObj
		}

		// Send the response
		responseJSON, err := json.Marshal(response)
		if err != nil {
			http.Error(w, fmt.Sprintf("{\"error\":\"Failed to marshal response: %s\"}", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(responseJSON)
		
		// Broadcast a notification about the command to all SSE clients in JSON-RPC format
		notification := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "notification",
			"params": map[string]interface{}{
				"type":    "command_executed",
				"command": jsonrpcRequest.Method,
				"time":    time.Now().Format(time.RFC3339),
			},
		}
		notificationJSON, _ := json.Marshal(notification)
		manager.broadcast <- notificationJSON
	}

	// Register the command handler for both endpoints with CORS middleware
	http.HandleFunc("/command", corsMiddleware(commandHandler))
	http.HandleFunc("/api/v1/mcp/command", corsMiddleware(commandHandler))

	// Add a simple status endpoint
	statusHandler := func(w http.ResponseWriter, r *http.Request) {
		// Handle HEAD requests for pre-connection checks
		if r.Method == "HEAD" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
			w.WriteHeader(http.StatusOK)
			return
		}
		
		status := map[string]interface{}{
			"status":    "running",
			"transport": "sse",
			"clients":   len(manager.clients),
			"uptime":    time.Since(time.Now()).String(),
		}
		
		statusJSON, _ := json.Marshal(status)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(statusJSON)
	}

	// Register the status handler for both endpoints with CORS middleware
	http.HandleFunc("/status", corsMiddleware(statusHandler))
	http.HandleFunc("/api/v1/mcp/status", corsMiddleware(statusHandler))

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
