package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// ClaudeCodeClient runs the Claude Code CLI (`claude -p`) as a subprocess
// and returns its output. Authentication relies on the CLI's own local
// login — no API key is read by this client. It exposes the same
// Chat(ctx, prompt) (string, error) shape as ClaudeClient, so it is a
// drop-in for the runtime's LLM calls.
//
// This is the default LLM backend for the in-memory runtime: it keeps the
// whole stack on Claude Code (consistent with the worker and loom) and
// needs no key, while the Gemini and Anthropic-API clients remain
// available for callers that prefer them.
type ClaudeCodeClient struct {
	binary string
	model  string
}

// NewClaudeCodeClient returns a Claude Code client. binary defaults to
// "claude"; model passes through to `claude --model` when non-empty
// (e.g. "sonnet", "opus").
func NewClaudeCodeClient(binary, model string) *ClaudeCodeClient {
	if binary == "" {
		binary = "claude"
	}
	return &ClaudeCodeClient{binary: binary, model: model}
}

// claudeCodeJSON is the subset of `claude -p --output-format json` we read.
type claudeCodeJSON struct {
	Result string `json:"result"`
	// The CLI emits more (cost, usage, etc.); we only need the result.
}

// Chat runs `claude -p <prompt> --output-format json` and returns the
// result text. It is a single completion, not an interactive agent loop.
func (c *ClaudeCodeClient) Chat(ctx context.Context, prompt string) (string, error) {
	args := []string{"-p", prompt, "--output-format", "json"}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}
	cmd := exec.CommandContext(ctx, c.binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude code: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	var parsed claudeCodeJSON
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		// Fall back to raw stdout if the CLI didn't emit JSON as expected.
		raw := strings.TrimSpace(stdout.String())
		if raw == "" {
			return "", fmt.Errorf("claude code: empty output (%w)", err)
		}
		return raw, nil
	}
	return strings.TrimSpace(parsed.Result), nil
}
