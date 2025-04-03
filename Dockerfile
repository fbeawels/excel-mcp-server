
FROM node:20-slim AS release

# Set the working directory
WORKDIR /app

RUN npm install -g @fbeawels/excel-mcp-server

# Set environment variables for SSE transport
ENV EXCEL_MCP_TRANSPORT=sse
ENV EXCEL_MCP_HOST=0.0.0.0
ENV EXCEL_MCP_PORT=8000

# Expose the port
EXPOSE 8000

# Command to run the application
ENTRYPOINT ["excel-mcp-server"]
