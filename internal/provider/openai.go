package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// openAICompatible speaks the OpenAI /chat/completions streaming (SSE) API.
// OpenAI, Fireworks, and Crusoe all expose this shape, so one adapter serves
// all three — they differ only in base_url, api key, and model names (all in
// models.yaml). Register each id so config stays declarative.
func init() {
	for _, id := range []string{"openai", "fireworks", "deepseek", "gemini"} {
		register(id, newOpenAICompatible)
	}
}

type openAICompatible struct {
	id      string
	baseURL string
	apiKey  string
	http    *http.Client
}

func newOpenAICompatible(id, baseURL, apiKey string) (Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("provider %q: empty api key", id)
	}
	return &openAICompatible{
		id:      id,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		// A generous client-level timeout backstops the per-call context so a
		// stalled connection can't hang a run indefinitely. CheckRedirect stops
		// a redirect from replaying the API key to another host.
		http: &http.Client{Timeout: 3 * time.Minute, CheckRedirect: noCrossHostRedirect},
	}, nil
}

func (c *openAICompatible) ID() string { return c.id }

// applyReasoning sets the provider-appropriate reasoning field on the request
// body and returns the level actually applied. The OpenAI-compatible shape is
// shared by several providers that DISAGREE on the field:
//   - OpenAI (gpt-5.x) and Fireworks (GLM/DeepSeek/MiniMax): reasoning_effort
//   - DeepSeek's own API: a top-level thinking:{type:"enabled"|"disabled"}
//
// "none" means "reasoning off"; any other value is passed through as the
// effort level (low/medium/high/...). An empty level leaves the request
// untouched (provider default).
func (c *openAICompatible) applyReasoning(payload map[string]any, level string) string {
	if level == "" {
		return "default"
	}
	if c.id == "deepseek" {
		// DeepSeek's native API uses a thinking toggle, not reasoning_effort.
		if level == "none" {
			payload["thinking"] = map[string]any{"type": "disabled"}
			return "none"
		}
		payload["thinking"] = map[string]any{"type": "enabled"}
		return level
	}
	// OpenAI + Fireworks: reasoning_effort accepts "none" to disable.
	payload["reasoning_effort"] = level
	return level
}

// oaiStreamChunk is the subset of a streamed chat.completion.chunk we read.
type oaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	// Usage is present on the final chunk when stream_options.include_usage is
	// set. Preferred source of the token counts.
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *openAICompatible) Stream(ctx context.Context, req Request) (StreamResult, error) {
	payload := map[string]any{
		"model":    req.Model,
		"messages": []map[string]string{{"role": "user", "content": req.Prompt}},
		"stream":   true,
		// Ask the server to include a usage block on the terminal chunk so we
		// get the provider's authoritative completion-token count.
		"stream_options": map[string]any{"include_usage": true},
	}
	// OpenAI's reasoning models (gpt-5.x) require max_completion_tokens and
	// reject a non-default temperature; the OpenAI-compatible open-model hosts
	// (Fireworks) still use the classic max_tokens + temperature. Branch on id.
	if c.id == "openai" {
		payload["max_completion_tokens"] = req.MaxTokens
	} else {
		payload["max_tokens"] = req.MaxTokens
		payload["temperature"] = req.Temperature
	}
	reasoningApplied := c.applyReasoning(payload, req.Reasoning)
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return StreamResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	start := time.Now()
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return StreamResult{}, fmt.Errorf("%s: request: %w", c.id, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return StreamResult{}, fmt.Errorf("%s: http %d: %s", c.id, resp.StatusCode, scrubError(string(snippet)))
	}

	var (
		res          StreamResult
		clientTokens int
		gotFirst     bool
	)
	sc := bufio.NewScanner(io.LimitReader(resp.Body, maxStreamBytes))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk oaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // tolerate keep-alives / partial lines
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			if !gotFirst {
				res.TimeToFirstToken = time.Since(start)
				gotFirst = true
			}
			clientTokens++ // fallback estimate: one delta ~ one token
		}
		if chunk.Usage != nil && chunk.Usage.CompletionTokens > 0 {
			res.OutputTokens = chunk.Usage.CompletionTokens
			res.InputTokens = chunk.Usage.PromptTokens
			res.UsageReported = true
		}
	}
	if err := sc.Err(); err != nil {
		return StreamResult{}, fmt.Errorf("%s: read stream: %w", c.id, err)
	}
	res.TotalDuration = time.Since(start)
	res.ReasoningApplied = reasoningApplied
	if !res.UsageReported {
		res.OutputTokens = clientTokens
	}
	if res.OutputTokens == 0 {
		return StreamResult{}, fmt.Errorf("%s: no output tokens received", c.id)
	}
	return res, nil
}
