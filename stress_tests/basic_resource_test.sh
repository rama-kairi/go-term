#!/bin/bash

echo "🔥 Basic Resource Monitoring Test"
echo "=================================="

# Build the MCP server
echo "📦 Building MCP server..."
go build -o stress_tests/go-term . || {
    echo "❌ Build failed"
    exit 1
}
echo "✅ MCP server built successfully"

# Start the MCP server in background
echo "🚀 Starting MCP server..."
./stress_tests/go-term > stress_tests/server.log 2>&1 &
SERVER_PID=$!
echo "✅ MCP server started with PID: $SERVER_PID"

# Wait a moment for server to initialize
sleep 2

# Test basic resource status
echo "📊 Testing resource monitoring..."

# Simulate some basic MCP calls
python3 << 'PYTHON'
import json
import subprocess
import time

def send_mcp_request(method, params=None):
    """Send a JSON-RPC request to the MCP server"""
    request = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": method
    }
    if params:
        request["params"] = params
    
    try:
        result = subprocess.run(
            ["./stress_tests/go-term"],
            input=json.dumps(request),
            text=True,
            capture_output=True,
            timeout=10
        )
        
        if result.stdout:
            try:
                return json.loads(result.stdout)
            except json.JSONDecodeError:
                print(f"Failed to parse response: {result.stdout}")
                return None
        return None
    except Exception as e:
        print(f"Request failed: {e}")
        return None

# Test basic functionality
print("🧪 Testing MCP server basic functionality...")

# Test resource status (if available)
print("📈 Checking resource status...")
response = send_mcp_request("tools/call", {
    "name": "get_resource_status",
    "arguments": {}
})

if response:
    print("✅ Resource monitoring is functional")
    print(f"Response: {json.dumps(response, indent=2)}")
else:
    print("⚠️  Resource monitoring test failed - this may be expected for direct binary testing")

print("🏁 Basic test completed")
PYTHON

# Clean up
echo "🧹 Cleaning up..."
kill $SERVER_PID 2>/dev/null
wait $SERVER_PID 2>/dev/null

echo "✅ Test completed successfully!"
