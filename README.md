# Excel MCP Server

<img src="docs/img/icon-800.png" width="128">

[![NPM Version](https://img.shields.io/npm/v/@fbeawels/excel-mcp-server)](https://www.npmjs.com/package/@fbeawels/excel-mcp-server)
[![smithery badge](https://smithery.ai/badge/@fbeawels/excel-mcp-server)](https://smithery.ai/server/@fbeawels/excel-mcp-server)

A Model Context Protocol (MCP) server that reads and writes MS Excel data.

## Features

- Read text values from MS Excel file
- Write text values to MS Excel file
- Read formulas from MS Excel file
- Write formulas to MS Excel file
- Capture screen image from MS Excel file (Windows only)

For more details, see the [tools](#tools) section.

## Requirements

- Node.js 20.x or later

## Supported file formats

- xlsx (Excel book)
- xlsm (Excel macro-enabled book)
- xltx (Excel template)
- xltm (Excel macro-enabled template)

## Installation

### Installing via NPM

excel-mcp-server is automatically installed by adding the following configuration to the MCP servers configuration.

#### Using stdio transport (default)

For Windows:
```json
{
    "mcpServers": {
        "excel": {
            "command": "cmd",
            "args": ["/c", "npx", "--yes", "@fbeawels/excel-mcp-server"],
            "env": {
                "EXCEL_MCP_PAGING_CELLS_LIMIT": "4000"
            }
        }
    }
}
```

For other platforms:
```json
{
    "mcpServers": {
        "excel": {
            "command": "npx",
            "args": ["--yes", "@fbeawels/excel-mcp-server"],
            "env": {
                "EXCEL_MCP_PAGING_CELLS_LIMIT": "4000"
            }
        }
    }
}
```

#### Using SSE transport

For Windows:
```json
{
    "mcpServers": {
        "excel": {
            "command": "cmd",
            "args": ["/c", "npx", "--yes", "@fbeawels/excel-mcp-server"],
            "env": {
                "EXCEL_MCP_PAGING_CELLS_LIMIT": "4000",
                "EXCEL_MCP_TRANSPORT": "sse",
                "EXCEL_MCP_HOST": "0.0.0.0",
                "EXCEL_MCP_PORT": "8000"
            }
        }
    }
}
```

For other platforms:
```json
{
    "mcpServers": {
        "excel": {
            "command": "npx",
            "args": ["--yes", "@fbeawels/excel-mcp-server"],
            "env": {
                "EXCEL_MCP_PAGING_CELLS_LIMIT": "4000",
                "EXCEL_MCP_TRANSPORT": "sse",
                "EXCEL_MCP_HOST": "0.0.0.0",
                "EXCEL_MCP_PORT": "8000"
            }
        }
    }
}
```

When using SSE transport, the server will start an HTTP server with the following endpoints:
- `/sse` - SSE endpoint for receiving events from the server
- `/command` - POST endpoint for sending commands to the server
- `/status` - GET endpoint for checking server status

### Installing via Smithery

To install Excel MCP Server for Claude Desktop automatically via [Smithery](https://smithery.ai/server/@fbeawels/excel-mcp-server):

```bash
npx -y @smithery/cli install @fbeawels/excel-mcp-server --client claude
```

<h2 id="tools">Tools</h2>

### `read_sheet_names`

List all sheet names in an Excel file.

**Arguments:**
- `fileAbsolutePath`
    - Absolute path to the Excel file

### `read_sheet_data`

Read data from Excel sheet with pagination.

**Arguments:**
- `fileAbsolutePath`
    - Absolute path to the Excel file
- `sheetName`
    - Sheet name in the Excel file
- `range`
    - Range of cells to read in the Excel sheet (e.g., "A1:C10"). [default: first paging range]
- `knownPagingRanges`
    - List of already read paging ranges

### `read_sheet_formula`

Read formulas from Excel sheet with pagination.

**Arguments:**
- `fileAbsolutePath`
    - Absolute path to the Excel file
- `sheetName`
    - Sheet name in the Excel file
- `range`
    - Range of cells to read in the Excel sheet (e.g., "A1:C10"). [default: first paging range]
- `knownPagingRanges`
    - List of already read paging ranges

### `read_sheet_image`

**[Windows only]** Read data as an image from the Excel sheet with pagination.

**Arguments:**
- `fileAbsolutePath`
    - Absolute path to the Excel file
- `sheetName`
    - Sheet name in the Excel file
- `range`
    - Range of cells to read in the Excel sheet (e.g., "A1:C10"). [default: first paging range]
- `knownPagingRanges`
    - List of already read paging ranges

### `write_sheet_data`

Write data to the Excel sheet.

**Arguments:**
- `fileAbsolutePath`
    - Absolute path to the Excel file
- `sheetName`
    - Sheet name in the Excel file
- `range`
    - Range of cells to read in the Excel sheet (e.g., "A1:C10").
- `data`
    - Data to write to the Excel sheet

### `write_sheet_formula`

Write formulas to the Excel sheet.

**Arguments:**
- `fileAbsolutePath`
    - Absolute path to the Excel file
- `sheetName`
    - Sheet name in the Excel file
- `range`
    - Range of cells to read in the Excel sheet (e.g., "A1:C10").
- `formulas`
    - Formulas to write to the Excel sheet (e.g., "=A1+B1")

<h2 id="tools">Configuration</h2>

### Command-line Arguments

The following command-line arguments are available:

```
--transport <type>   Transport type (stdio or sse)
--host <address>     Host address to listen on for SSE transport (default: all interfaces)
--port <number>      Port for SSE transport (default: 8000)
```

Example usage:
```bash
npx @fbeawels/excel-mcp-server --transport sse --host 0.0.0.0 --port 8000
```

### Environment Variables

You can also change the MCP Server behaviors by the following environment variables:

### `EXCEL_MCP_PAGING_CELLS_LIMIT`

The maximum number of cells to read in a single paging operation.  
[default: 4000]

### `EXCEL_MCP_TRANSPORT`

The transport mechanism to use for communication.  
Possible values: `stdio`, `sse`  
[default: stdio]

### `EXCEL_MCP_HOST`

The host address to bind to when using SSE transport.  
[default: ""] (all interfaces)

### `EXCEL_MCP_PORT`

The port to use for the HTTP server when using SSE transport.  
[default: 8000]

## License

Copyright (c) 2025 Franck Besnard

excel-mcp-server is released under the [MIT License](LICENSE)
