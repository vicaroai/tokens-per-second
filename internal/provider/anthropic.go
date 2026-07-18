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

// anthropic speaks the native Anthropic Messages streaming API, which differs
// from the OpenAI shape: a /messages endpoint, x-api-key + anthropic-version
// headers, and SSE events (message_start, content_block_delta, message_delta)
// rather than chat.completion chunks.
func init() { register("anthropic", newAnthropic) }

const anthropicVersion = "2023-06-01"

type anthropic struct {
	id      string
	baseURL string
	apiKey  string
	http    *http.Client
}

func newAnthropic(id, baseURL, apiKey string) (Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("provider %q: empty api key", id)
	}
	return &anthropic{
		id:      id,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 3 * time.Minute},
	}, nil
}

func (c *anthropic) ID() string { return c.id }

// applyAnthropicReasoning sets the thinking field and returns the level
// actually applied. Verified empirically against the live API (the published
// docs were inaccurate on these points):
//   - thinking:{type:"disabled"} IS accepted and genuinely suppresses reasoning
//     (response has no thinking block) — so "none"/"minimal" fully disable it.
//   - thinking.adaptive.effort and a top-level effort are both REJECTED (400).
//   - Any other level enables adaptive thinking (thinking:{type:"adaptive"});
//     Anthropic decides the depth (no per-request effort knob is accepted).
//
// Note: temperature is intentionally omitted — Anthropic rejects a non-default
// temperature when thinking is enabled.
func applyAnthropicReasoning(payload map[string]any, level string) string {
	if level == "" || level == "none" || level == "minimal" {
		payload["thinking"] = map[string]any{"type": "disabled"}
		return "none"
	}
	payload["thinking"] = map[string]any{"type": "adaptive"}
	return "adaptive"
}

// anthropicEvent is the subset of the streamed SSE payloads we read. Anthropic
// reports output_tokens on message_start (partial) and message_delta (final),
// so we take the last value seen.
type anthropicEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Text string `json:"text"`
	} `json:"delta"`
	Message struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (c *anthropic) Stream(ctx context.Context, req Request) (StreamResult, error) {
	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"stream":     true,
		"messages":   []map[string]string{{"role": "user", "content": req.Prompt}},
	}
	reasoningApplied := applyAnthropicReasoning(payload, req.Reasoning)
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return StreamResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

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
		res      StreamResult
		gotFirst bool
	)
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var ev anthropicEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Text != "" && !gotFirst {
				res.TimeToFirstToken = time.Since(start)
				gotFirst = true
			}
		case "message_start":
			if ev.Message.Usage.InputTokens > 0 {
				res.InputTokens = ev.Message.Usage.InputTokens
			}
			if ev.Message.Usage.OutputTokens > 0 {
				res.OutputTokens = ev.Message.Usage.OutputTokens
				res.UsageReported = true
			}
		case "message_delta":
			if ev.Usage.OutputTokens > 0 {
				res.OutputTokens = ev.Usage.OutputTokens
				res.UsageReported = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return StreamResult{}, fmt.Errorf("%s: read stream: %w", c.id, err)
	}
	res.TotalDuration = time.Since(start)
	res.ReasoningApplied = reasoningApplied
	if res.OutputTokens == 0 {
		return StreamResult{}, fmt.Errorf("%s: no output tokens received", c.id)
	}
	return res, nil
}
