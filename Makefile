.PHONY: run build test docker clean

run:
	go run main.go

build:
	CGO_ENABLED=1 go build -o bin/agentguard .
	@echo "Binary: bin/agentguard"

test:
	go test ./...

docker:
	docker build -t agentguard:latest .
	@echo "Image: agentguard:latest"

clean:
	rm -rf bin/ audit.db

# Run full end-to-end smoke test
smoke-test:
	@bash scripts/smoke-test.sh

smoke-test-prod:
	@GUARD_URL=https://agentguard.fly.dev bash scripts/smoke-test.sh

# Test an MCP block (no auth needed, just starts the server)
test-block:
	curl -s -X POST http://localhost:7777/mcp \
	  -H "Content-Type: application/json" \
	  -H "X-Agent-ID: test-agent" \
	  -d '{"jsonrpc":"2.0","id":"1","method":"tools/call","params":{"name":"delete_file","arguments":{"path":"/data"}}}' \
	  | jq .

# Test an MCP allow
test-allow:
	curl -s -X POST http://localhost:7777/mcp \
	  -H "Content-Type: application/json" \
	  -H "X-Agent-ID: test-agent" \
	  -d '{"jsonrpc":"2.0","id":"2","method":"tools/call","params":{"name":"read_file","arguments":{"path":"/data"}}}' \
	  | jq .

# Reload policy without restart
reload:
	curl -s -X POST http://localhost:7777/reload-policy | jq .

# Health check
health:
	curl -s http://localhost:7777/health | jq .

# Test an MCP escalate
test-escalate:
	curl -s -X POST http://localhost:7777/mcp \
	  -H "Content-Type: application/json" \
	  -H "X-Agent-ID: billing-agent" \
	  -d '{"jsonrpc":"2.0","id":"3","method":"tools/call","params":{"name":"send_payment","arguments":{"amount":500}}}' \
	  | jq .

list-escalations:
	curl -s -H "X-AgentGuard-Admin-Key: test_admin_key" http://localhost:7777/escalations | jq .

check-spend:
	curl -s -X POST http://localhost:7777/atxp \
	  -H "Content-Type: application/json" \
	  -H "X-Agent-ID: test-agent" \
	  -d '{"tool":"some_paid_tool","arguments":{"amount":{"value":"2.00"}}}' \
	  | jq .

test-spend-limit:
	for i in 1 2 3 4 5; do \
	  curl -s -X POST http://localhost:7777/mcp \
	    -H "Content-Type: application/json" \
	    -H "X-Agent-ID: billing-agent" \
	    -d "{\"jsonrpc\":\"2.0\",\"id\":\"$$i\",\"method\":\"tools/call\",\"params\":{\"name\":\"read_invoice\",\"arguments\":{}}}" \
	    | jq .decision; \
	done

# Run Python SDK tests
test-python:
	cd sdk/python && pip install -e ".[dev]" -q && pytest tests/ -v

# Run Node SDK tests
test-node:
	cd sdk/node && npm install -q && npm test

# Validate the example policy
validate-policy:
	cd sdk/python && python3 -c "\
from agentguard.policy import load_policy, validate_policy; \
p = load_policy('../../examples/policy.yaml'); \
errors = validate_policy(p); \
print('✓ Policy valid' if not errors else '\n'.join(errors))"

# Build all SDK packages
build-sdks: build-python build-node

build-python:
	cd sdk/python && pip install build -q && python -m build

build-node:
	cd sdk/node && npm install -q && npm run build

# Tag and push a release
release:
	@echo "Usage: make release VERSION=0.1.0"
	git tag v$(VERSION)
	git push origin v$(VERSION)

# Simulate a decision using the Python SDK
simulate:
	@echo "Usage: make simulate AGENT=billing-agent TOOL=send_payment"
	cd sdk/python && python3 -c "\
from agentguard.policy import load_policy, simulate_decision; \
p = load_policy('../../examples/policy.yaml'); \
d = simulate_decision(p, '$(AGENT)', '$(TOOL)'); \
print(f'Decision for $(AGENT) → $(TOOL): {d}')"

# Build the CLI binary
build-cli:
	CGO_ENABLED=1 go build \
	  -ldflags="-w -s -X main.version=$$(git describe --tags --always 2>/dev/null || echo '0.1.0')" \
	  -o bin/agentguard ./cmd/cli

# Install CLI globally (to /usr/local/bin)
install-cli: build-cli
	sudo cp bin/agentguard /usr/local/bin/agentguard
	@echo "✓ agentguard installed to /usr/local/bin"
	@agentguard version

# Run CLI directly (dev)
cli:
	go run ./cmd/cli $(ARGS)

