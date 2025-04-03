#!/usr/bin/env python3
import asyncio
import json
import logging
from contextlib import AsyncExitStack

import httpx
from httpx_sse import aconnect_sse

# Configure logging
logging.basicConfig(
    level=logging.DEBUG,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger("mcp_test_client")

async def test_sse_connection(url):
    """Test a connection to an MCP SSE server."""
    logger.info(f"Connecting to MCP SSE server at {url}")
    
    # Create an exit stack for managing async resources
    async with AsyncExitStack() as stack:
        # Create HTTP client
        client = httpx.AsyncClient()
        await stack.enter_async_context(client)
        
        # Connect to SSE endpoint
        logger.info("Establishing SSE connection...")
        event_source = await stack.enter_async_context(
            aconnect_sse(client, "GET", url, timeout=httpx.Timeout(60.0))
        )
        
        # Check response status
        event_source.response.raise_for_status()
        logger.info(f"Connection established: {event_source.response.status_code}")
        
        # Process SSE events
        endpoint_url = None
        
        logger.info("Waiting for events...")
        async for sse in event_source.aiter_sse():
            logger.info(f"Received event type: {sse.event}")
            logger.info(f"Event data: {sse.data}")
            
            if sse.event == "endpoint":
                endpoint_url = sse.data
                logger.info(f"Received endpoint URL: {endpoint_url}")
                
                # Test sending a message to the endpoint
                full_endpoint = f"{url.rsplit('/', 1)[0]}{endpoint_url}"
                logger.info(f"Sending initialize message to {full_endpoint}")
                
                # Create initialize message
                initialize_msg = {
                    "jsonrpc": "2.0",
                    "method": "initialize",
                    "params": {
                        "protocolVersion": "2024-04-03",
                        "clientInfo": {
                            "name": "test-client",
                            "version": "1.0.0"
                        },
                        "capabilities": {
                            "tools": True
                        }
                    },
                    "id": 1
                }
                
                # Send initialize message
                response = await client.post(
                    full_endpoint,
                    json=initialize_msg
                )
                
                logger.info(f"Initialize response status: {response.status_code}")
                if response.status_code == 200:
                    logger.info("Initialize message sent successfully")
                else:
                    logger.error(f"Failed to send initialize message: {response.text}")
            
            elif sse.event == "message":
                try:
                    message = json.loads(sse.data)
                    logger.info(f"Received message: {json.dumps(message, indent=2)}")
                    
                    # If we receive a response to our initialize request
                    if message.get("id") == 1 and "result" in message:
                        logger.info("Received initialize response")
                        
                        # Test list_tools
                        if endpoint_url:
                            full_endpoint = f"{url.rsplit('/', 1)[0]}{endpoint_url}"
                            logger.info(f"Sending list_tools message to {full_endpoint}")
                            
                            list_tools_msg = {
                                "jsonrpc": "2.0",
                                "method": "list_tools",
                                "id": 2
                            }
                            
                            response = await client.post(
                                full_endpoint,
                                json=list_tools_msg
                            )
                            
                            logger.info(f"list_tools response status: {response.status_code}")
                            if response.status_code == 200:
                                logger.info("list_tools message sent successfully")
                            else:
                                logger.error(f"Failed to send list_tools message: {response.text}")
                except json.JSONDecodeError:
                    logger.error(f"Failed to parse message as JSON: {sse.data}")
            
            # For demonstration purposes, limit the number of events we process
            if endpoint_url and sse.event == "message":
                # Wait for a few more messages to see heartbeats
                await asyncio.sleep(5)
                logger.info("Test completed successfully")
                return True
        
        return False

async def main():
    # Test connection to the MCP SSE server
    url = "http://localhost:10000/api/v1/mcp/sse"
    try:
        success = await test_sse_connection(url)
        if success:
            logger.info("MCP SSE connection test passed!")
        else:
            logger.error("MCP SSE connection test failed!")
    except Exception as e:
        logger.error(f"Error during test: {e}")
        raise

if __name__ == "__main__":
    asyncio.run(main())
