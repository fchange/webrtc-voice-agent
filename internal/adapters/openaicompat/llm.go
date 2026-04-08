package openaicompat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
)

type LLM struct {
	cfg    config.OpenAICompatibleLLMConfig
	client *http.Client
	logger *slog.Logger
}

func NewLLM(cfg config.OpenAICompatibleLLMConfig, logger *slog.Logger) *LLM {
	if logger == nil {
		logger = slog.Default()
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 0
	}
	return &LLM{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
		logger: logger.With("provider", "openai-compatible-chat-completions"),
	}
}

func (l *LLM) Name() string {
	return "openai-compatible-chat-completions"
}

func (l *LLM) Ready() bool {
	return l.cfg.BaseURL != "" && l.cfg.APIKey != "" && l.cfg.Model != ""
}

func (l *LLM) Complete(ctx context.Context, req adapters.CompletionRequest) (<-chan adapters.LLMEvent, error) {
	if !l.Ready() {
		return nil, fmt.Errorf("openai-compatible llm credentials are incomplete")
	}

	body, err := json.Marshal(chatCompletionRequest{
		Model: l.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: l.cfg.SystemPrompt},
			{Role: "user", Content: req.Text},
		},
		Stream:      true,
		MaxTokens:   l.cfg.MaxTokens,
		Temperature: l.cfg.Temperature,
		TopP:        l.cfg.TopP,
		TopK:        l.cfg.TopK,
	})
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, l.cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+l.cfg.APIKey)
	if l.cfg.FailoverEnabled {
		httpReq.Header.Set("X-Failover-Enabled", "true")
	}

	resp, err := l.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openai-compatible llm status=%s body=%s", resp.Status, strings.TrimSpace(string(data)))
	}

	out := make(chan adapters.LLMEvent, 32)
	go l.readStream(resp.Body, out)
	return out, nil
}

func (l *LLM) readStream(body io.ReadCloser, out chan<- adapters.LLMEvent) {
	defer close(out)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			out <- adapters.LLMEvent{Final: true}
			return
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			l.logger.Error("decode openai-compatible llm chunk failed", "err", err, "payload", payload)
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta.Content
		finishReason := chunk.Choices[0].FinishReason
		if delta != "" {
			l.logger.Info("openai-compatible llm token received", "len", len(delta), "text", delta)
			out <- adapters.LLMEvent{Text: delta}
		}
		if finishReason != "" {
			l.logger.Info("openai-compatible llm stream finished", "finish_reason", finishReason)
			out <- adapters.LLMEvent{Final: true}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		l.logger.Error("read openai-compatible llm stream failed", "err", err)
		return
	}

	out <- adapters.LLMEvent{Final: true}
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
	TopK        int           `json:"top_k,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionChunk struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Delta        chatDelta `json:"delta"`
	FinishReason string    `json:"finish_reason"`
}

type chatDelta struct {
	Content string `json:"content"`
}
