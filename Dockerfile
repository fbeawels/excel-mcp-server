
FROM golang:1.24 AS go-build

# Set the working directory
WORKDIR /go-build

# Copy Go source code
COPY . .

# Build the Go binary directly
RUN mkdir -p dist/excel-mcp-server_linux_amd64_v1 && \
    cd cmd/excel-mcp-server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../dist/excel-mcp-server_linux_amd64_v1/excel-mcp-server .

FROM node:20-slim AS node-build

# Set the working directory
WORKDIR /node-build

# Copy package files
COPY package.json package-lock.json ./

# Copy the rest of the application
COPY . .

# Copy the built Go binary from the go-build stage
COPY --from=go-build /go-build/dist ./dist

# Install dependencies and build TypeScript
RUN npm install
RUN npx tsc

FROM node:20-slim AS release

# Set the working directory
WORKDIR /app

# Copy built application from build stage
COPY --from=node-build /node-build/dist ./dist
COPY --from=node-build /node-build/package.json ./
COPY --from=node-build /node-build/package-lock.json ./
COPY --from=node-build /node-build/node_modules ./node_modules

# Set environment variables for SSE transport
ENV EXCEL_MCP_TRANSPORT=sse
ENV EXCEL_MCP_HOST=0.0.0.0
ENV EXCEL_MCP_PORT=8000

# Expose the port
EXPOSE 8000

# Command to run the application
ENTRYPOINT ["node", "dist/launcher.js"]
