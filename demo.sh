#!/bin/bash
# XPOS Local K8s Demo Script
# This script starts a local HTTP server, port-forwards to the relay,
# runs the agent, and sets up Envoy Gateway port-forward for testing.

set -e

DEMO_DIR="/tmp/xpos-demo"
AGENT_PID=""
SERVER_PID=""
PF_RELAY_PID=""
PF_EG_PID=""

cleanup() {
    echo ""
    echo "=== Cleaning up ==="
    [ -n "$AGENT_PID" ] && kill $AGENT_PID 2>/dev/null || true
    [ -n "$SERVER_PID" ] && kill $SERVER_PID 2>/dev/null || true
    pkill -f "port-forward.*9876" 2>/dev/null || true
    pkill -f "port-forward.*18080" 2>/dev/null || true
    echo "Done!"
}
trap cleanup EXIT

# Create demo server
mkdir -p "$DEMO_DIR"
cat > "$DEMO_DIR/server.py" << 'EOF'
#!/usr/bin/env python3
import http.server
import socketserver
import sys
import json

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8888

class Handler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/html')
        self.end_headers()
        response = f"""
        <h1>XPOS Tunnel Working!</h1>
        <p><strong>Request path:</strong> {self.path}</p>
        <p><strong>Headers:</strong></p>
        <pre>{json.dumps(dict(self.headers), indent=2)}</pre>
        <p><strong>Time:</strong> {__import__('datetime').datetime.now()}</p>
        """
        self.wfile.write(response.encode())

    def log_message(self, format, *args):
        print(f"[LOCAL SERVER] {args[0]}")

with socketserver.TCPServer(("", PORT), Handler) as httpd:
    print(f"Local server running at http://localhost:{PORT}")
    httpd.serve_forever()
EOF

echo "=== XPOS Local K8s Tunnel Demo ==="
echo ""

# 1. Start local server
# echo "1. Starting local HTTP server on port 8888..."
# cd "$DEMO_DIR"
# python3 server.py 8888 > /tmp/xpos_server.log 2>&1 &
# SERVER_PID=$!
# sleep 2
# echo "   ✓ Local server: http://localhost:8888 (PID: $SERVER_PID)"
# echo ""

# 2. Port-forward to relay
echo "2. Setting up port-forward to relay events port..."
kubectl -n xpos-system port-forward xpos-relay-0 9876:9876 > /tmp/xpos_pf_relay.log 2>&1 &
PF_RELAY_PID=$!
sleep 3
echo "   ✓ Port-forward: localhost:9876 -> xpos-relay-0:9876"
echo ""

# 3. Run agent
echo "3. Starting agent tunnel..."
cd /Users/arsinux/projects/go/xpos
go run ./cmd/agent http 8888 &
AGENT_PID=$!
sleep 5
echo "   ✓ Agent running (PID: $AGENT_PID)"
echo ""

# 4. Check resources
echo "4. Checking Kubernetes resources..."
echo "   Agents:"
kubectl -n xpos-system get agents 2>&1 | sed 's/^/     /'
echo ""
echo "   Tunnels:"
kubectl -n xpos-system get tunnels 2>&1 | sed 's/^/     /'
echo ""
echo "   HTTPRoutes:"
kubectl -n xpos-system get httproute 2>&1 | sed 's/^/     /'
echo ""

# 5. Port-forward to Envoy Gateway
echo "5. Setting up Envoy Gateway port-forward..."
# Get the Envoy Gateway service name dynamically
EG_SVC=$(kubectl -n envoy-gateway-system get svc -o name | grep envoy-xpos-system | head -1 | sed 's/service\///')
if [ -z "$EG_SVC" ]; then
    echo "   ✗ Envoy Gateway service not found!"
    echo "   Available services:"
    kubectl -n envoy-gateway-system get svc | sed 's/^/     /'
    cleanup
    exit 1
fi
echo "   Using service: $EG_SVC"
kubectl -n envoy-gateway-system port-forward service/$EG_SVC 18080:80 > /tmp/xpos_pf_eg.log 2>&1 &
PF_EG_PID=$!
sleep 3
echo "   ✓ Envoy Gateway: localhost:18080"
echo ""

echo "=================================="
echo "🚀 TUNNEL IS LIVE!"
echo "=================================="
echo ""
echo "Test commands:"
echo "   curl -H 'Host: dev.xpos-it.com' http://localhost:18080/"
echo "   curl -H 'Host: dev.xpos-it.com' http://localhost:18080/api/test"
echo "   curl -H 'Host: dev.xpos-it.com' http://localhost:18080/hello"
echo ""
echo "Direct local server:"
echo "   curl http://localhost:8888/"
echo ""
echo "Press Ctrl+C to stop"
echo ""

# Wait for interrupt
wait $AGENT_PID 2>/dev/null || true
