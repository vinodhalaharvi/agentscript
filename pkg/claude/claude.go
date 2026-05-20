package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ClaudeClient handles Anthropic Claude API
type ClaudeClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewClaudeClient creates a new Claude API client
func NewClaudeClient(apiKey string) *ClaudeClient {
	model := "claude-opus-4-7"
	return &ClaudeClient{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{},
	}
}

// Message represents a Claude message
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Chat sends a message to Claude and returns the response
func (c *ClaudeClient) Chat(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model":      c.model,
		"max_tokens": 32000,
		"messages": []claudeMessage{
			{Role: "user", Content: prompt},
		},
	}
	return c.doRequest(ctx, reqBody)
}

// Session maintains a multi-turn conversation with Claude
type Session struct {
	client       *ClaudeClient
	messages     []claudeMessage
	SystemPrompt string // persistent system prompt — never dropped
	TotalInput   int    // cumulative input tokens
	TotalOutput  int    // cumulative output tokens
	CallCount    int    // number of API calls
	LastInput    int    // last call input tokens
	LastOutput   int    // last call output tokens
}

// NewSession creates a new conversational session
func (c *ClaudeClient) NewSession() *Session {
	return &Session{
		client:   c,
		messages: make([]claudeMessage, 0),
	}
}

// Chat sends a message in the session and returns the response.
// Maintains conversation history — Claude remembers previous turns.
func (s *Session) Chat(ctx context.Context, prompt string) (string, error) {
	// Add user message to history
	s.messages = append(s.messages, claudeMessage{Role: "user", Content: prompt})

	reqBody := map[string]interface{}{
		"model":      s.client.model,
		"max_tokens": 32000,
		"messages":   s.messages,
	}

	// System prompt is always sent — never dropped from context
	if s.SystemPrompt != "" {
		reqBody["system"] = s.SystemPrompt
	}

	response, usage, err := s.client.doRequestWithUsage(ctx, reqBody)
	if err != nil {
		// Remove the failed user message
		s.messages = s.messages[:len(s.messages)-1]
		return "", err
	}

	// Track tokens
	s.CallCount++
	if usage != nil {
		s.LastInput = usage.InputTokens
		s.LastOutput = usage.OutputTokens
		s.TotalInput += usage.InputTokens
		s.TotalOutput += usage.OutputTokens
	}

	// Add assistant response to history
	s.messages = append(s.messages, claudeMessage{Role: "assistant", Content: response})

	return response, nil
}

// MessageCount returns the number of messages in the session
func (s *Session) MessageCount() int {
	return len(s.messages)
}

// CompactOldAssistantMessages replaces the content of all assistant messages
// except the most recent `keepLast` with a short placeholder. User messages
// are left untouched.
//
// Motivation: in converge loops the assistant typically returns large JSON
// proposals (file contents, patches, commands). Those are consumed by the
// engine immediately and their full text is never needed again — the repo
// state on disk is the source of truth. Keeping them in s.messages means
// every subsequent Chat() call re-uploads them as input tokens, so cost
// and latency grow linearly with loop count.
//
// This is called by the converge engine after each accepted proposal to
// hold the in-flight context to roughly O(keepLast) large responses
// instead of O(N).
//
// keepLast < 1 is clamped to 1 (we always preserve the most recent
// assistant response so Claude can reference its own last decision).
func (s *Session) CompactOldAssistantMessages(keepLast int) {
	if keepLast < 1 {
		keepLast = 1
	}

	// Find indices of assistant messages.
	var assistantIdx []int
	for i, m := range s.messages {
		if m.Role == "assistant" {
			assistantIdx = append(assistantIdx, i)
		}
	}
	if len(assistantIdx) <= keepLast {
		return
	}

	// Replace all but the last `keepLast` assistant messages with a
	// placeholder. The placeholder preserves the turn-taking structure
	// (Anthropic requires alternating user/assistant) while shrinking
	// the payload.
	cutoff := len(assistantIdx) - keepLast
	for i := 0; i < cutoff; i++ {
		idx := assistantIdx[i]
		original := len(s.messages[idx].Content)
		s.messages[idx].Content = fmt.Sprintf(
			"[prior response elided — %d chars; changes already applied to the sandbox]",
			original,
		)
	}
}

// TokenSummary returns a formatted string of token usage
func (s *Session) TokenSummary() string {
	return fmt.Sprintf("call %d: %d in / %d out (total: %d in / %d out)",
		s.CallCount, s.LastInput, s.LastOutput, s.TotalInput, s.TotalOutput)
}

// TokenUsage tracks tokens consumed per API call
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// doRequest makes the actual API call and returns response + token usage
func (c *ClaudeClient) doRequest(ctx context.Context, reqBody map[string]interface{}) (string, error) {
	text, _, err := c.doRequestWithUsage(ctx, reqBody)
	return text, err
}

func (c *ClaudeClient) doRequestWithUsage(ctx context.Context, reqBody map[string]interface{}) (string, *TokenUsage, error) {

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return "", nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", nil, fmt.Errorf("claude API error: status %d - %s", resp.StatusCode, string(body))
	}

	var claudeResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage *TokenUsage `json:"usage"`
	}

	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return "", nil, fmt.Errorf("no content in response")
	}

	return claudeResp.Content[0].Text, claudeResp.Usage, nil
}
