# AgentScript Makefile

# Load .env file if it exists
ifneq (,$(wildcard .env))
    include .env
    export
endif

BINARY=agentscript

.DEFAULT_GOAL := help

# === Build and check =======================================================

build:
	go build -o $(BINARY) ./cmd/agentscript/

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

# Everything CI runs, in one target.
check: fmt vet test

# === Running programs ======================================================

# The file is a positional argument; -e takes program text. There is no
# -f, -i or -n: the REPL and natural-language modes went with the legacy
# CLI when the two binaries collapsed into one.
run: build
	./$(BINARY) -e '$(EXPR)'

run-file: build
	./$(BINARY) $(FILE)

# Compile a temporal program and print its Plan as JSON. Needs no
# Temporal cluster and no worker, which makes it the quickest way to see
# what the front end produced.
dry-run: build
	./$(BINARY) --dry-run $(FILE)

# === Examples ==============================================================

# hello is the only example that runs with no credentials and no
# infrastructure; the rest need API keys, and durable-echo needs a
# Temporal cluster plus a Sibyl worker on the sibyl-agents queue.
example-hello: build
	./$(BINARY) examples/hello.as

example-pipeline: build
	./$(BINARY) examples/research.as

example-fanout: build
	./$(BINARY) examples/tech-digest.as

example-nested: build
	./$(BINARY) examples/framework-compare.as

example-mcp: build
	./$(BINARY) examples/mcp-issues.as

# Compiles without infrastructure; drop --dry-run once a worker is up.
example-durable: build
	./$(BINARY) --dry-run examples/durable-echo.as

# Parse every example without running any of them. --dry-run stops
# before execution, and a parse failure is the only error that can
# mention the Parse phase, so grepping for it separates "bad syntax"
# from "this example needs credentials".
examples: build
	@fail=0; for f in examples/*.as; do \
		printf '%-32s ' "$$f"; \
		if ./$(BINARY) --dry-run "$$f" 2>&1 | grep -q 'script.Parse'; then \
			echo "PARSE FAILED"; fail=1; \
		else \
			echo ok; \
		fi; \
	done; exit $$fail

# === Housekeeping ==========================================================

clean:
	rm -f $(BINARY)

deps:
	go mod tidy

help:
	@echo "AgentScript — AI agent orchestration in s-expressions"
	@echo ""
	@echo "Setup:"
	@echo "  1. Create .env with the keys the verbs you use need, e.g. GEMINI_API_KEY"
	@echo "  2. make build"
	@echo ""
	@echo "Build and check:"
	@echo "  make build              Build the binary"
	@echo "  make test               go test ./..."
	@echo "  make vet                go vet ./..."
	@echo "  make fmt                List files needing gofmt"
	@echo "  make check              fmt + vet + test"
	@echo ""
	@echo "Run:"
	@echo "  make run EXPR='(pipe (search \"go\") summarize)'"
	@echo "  make run-file FILE=examples/tech-digest.as"
	@echo "  make dry-run FILE=examples/durable-echo.as"
	@echo ""
	@echo "Examples:"
	@echo "  make example-hello      exec, no credentials needed"
	@echo "  make example-pipeline   (pipe ...)"
	@echo "  make example-fanout     (par ...) + merge"
	@echo "  make example-nested     nested fan-out"
	@echo "  make example-mcp        MCP server tools"
	@echo "  make example-durable    temporal backend, compiled not run"
	@echo "  make examples           Parse every example"
	@echo ""
	@echo "Syntax:"
	@echo "  Sequential : (pipe (search \"golang\") summarize (save \"out.md\"))"
	@echo "  Fan-out    : (pipe (par (search \"AWS\") (search \"GCP\")) merge)"
	@echo "  Conditional: (pipe (stock \"NVDA\") (when \"change > 5\") (notify \"slack\"))"
	@echo "  Backend    : (block :backend temporal :mode static (echo \"hi\"))"
	@echo ""
	@echo "Forms:"
	@echo "  (pipe a b c)   each stage's output feeds the next"
	@echo "  (par a b c)    branches run concurrently on the same input"
	@echo "  (block ...)    optional wrapper; defaults to :backend memory :mode static"
	@echo ""
	@echo "Running a temporal program needs a Temporal cluster and a Sibyl"
	@echo "worker on the sibyl-agents queue:"
	@echo "  temporal server start-dev"
	@echo "  go run ./cmd/worker          # from your sibyl checkout"

.PHONY: build test vet fmt check run run-file dry-run \
	example-hello example-pipeline example-fanout example-nested \
	example-mcp example-durable examples clean deps help
