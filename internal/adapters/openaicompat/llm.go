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

	"github.com/fchange/webrtc-voice-agent/internal/adapters"
	"github.com/fchange/webrtc-voice-agent/internal/config"
)

type LLM struct {
	cfg        config.OpenAICompatibleLLMConfig
	client     *http.Client
	logger     *slog.Logger
	tools      []Tool
	finalizers []ToolFinalizer
}

type ToolHandler func(ctx context.Context, arguments json.RawMessage) (any, error)
type ToolPredicate func(req adapters.CompletionRequest) bool
type ToolFinalizer func(content string, results []ToolCallResult) string

type ToolCallResult struct {
	Name    string
	Content string
}

type Tool struct {
	Type      string        `json:"type"`
	Function  ToolFunction  `json:"function"`
	Handler   ToolHandler   `json:"-"`
	ShouldUse ToolPredicate `json:"-"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
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

func (l *LLM) WithTools(tools []Tool) *LLM {
	next := *l
	next.tools = append([]Tool(nil), tools...)
	return &next
}

func (l *LLM) WithToolFinalizers(finalizers []ToolFinalizer) *LLM {
	next := *l
	next.finalizers = append([]ToolFinalizer(nil), finalizers...)
	return &next
}

func (l *LLM) Ready() bool {
	return l.cfg.BaseURL != "" && l.cfg.APIKey != "" && l.cfg.Model != ""
}

func (l *LLM) Complete(ctx context.Context, req adapters.CompletionRequest) (<-chan adapters.LLMEvent, error) {
	if !l.Ready() {
		return nil, fmt.Errorf("openai-compatible llm credentials are incomplete")
	}

	messages := []chatMessage{
		{Role: "system", Content: joinSystemPrompt(l.cfg.SystemPrompt, req.SystemHint)},
	}
	for _, item := range req.History {
		role := strings.TrimSpace(item.Role)
		text := strings.TrimSpace(item.Text)
		if role == "" || text == "" {
			continue
		}
		if role != "user" && role != "assistant" {
			continue
		}
		messages = append(messages, chatMessage{Role: role, Content: text})
	}
	messages = append(messages, chatMessage{Role: "user", Content: req.Text})
	maxTokens := l.maxTokensForRequest(req)

	tools := l.toolsForRequest(req)
	if len(tools) > 0 {
		return l.completeWithTools(ctx, messages, tools, maxTokens)
	}

	return l.streamCompletion(ctx, messages, maxTokens)
}

func joinSystemPrompt(base string, hint string) string {
	base = strings.TrimSpace(base)
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return base
	}
	if base == "" {
		return hint
	}
	return base + "\n\n" + hint
}

func (l *LLM) toolsForRequest(req adapters.CompletionRequest) []Tool {
	tools := make([]Tool, 0, len(l.tools))
	for _, tool := range l.tools {
		if tool.ShouldUse == nil || tool.ShouldUse(req) {
			tools = append(tools, tool)
		}
	}
	return tools
}

func (l *LLM) maxTokensForRequest(req adapters.CompletionRequest) int {
	maxTokens := l.cfg.MaxTokens
	if strings.TrimSpace(req.SystemHint) != "" && (maxTokens == 0 || maxTokens > 96) {
		return 96
	}
	return maxTokens
}

func (l *LLM) streamCompletion(ctx context.Context, messages []chatMessage, maxTokens int) (<-chan adapters.LLMEvent, error) {
	resp, err := l.sendChatRequest(ctx, chatCompletionRequest{
		Model:       l.cfg.Model,
		Messages:    messages,
		Stream:      true,
		MaxTokens:   maxTokens,
		Temperature: l.cfg.Temperature,
		TopP:        l.cfg.TopP,
		TopK:        l.cfg.TopK,
	})
	if err != nil {
		return nil, err
	}

	out := make(chan adapters.LLMEvent, 32)
	go l.readStream(resp.Body, out)
	return out, nil
}

const maxToolIterations = 6

func (l *LLM) completeWithTools(ctx context.Context, messages []chatMessage, tools []Tool, maxTokens int) (<-chan adapters.LLMEvent, error) {
	var toolResults []ToolCallResult

	for i := 0; i < maxToolIterations; i++ {
		resp, err := l.sendChatRequest(ctx, chatCompletionRequest{
			Model:       l.cfg.Model,
			Messages:    messages,
			Stream:      false,
			MaxTokens:   maxTokens,
			Temperature: l.cfg.Temperature,
			TopP:        l.cfg.TopP,
			TopK:        l.cfg.TopK,
			Tools:       toolDefinitions(tools),
			ToolChoice:  "auto",
		})
		if err != nil {
			return nil, err
		}

		var decoded chatCompletionResponse
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("decode openai-compatible llm response: %w", err)
		}
		_ = resp.Body.Close()

		if len(decoded.Choices) == 0 {
			return nil, fmt.Errorf("openai-compatible llm returned no choices")
		}

		message := decoded.Choices[0].Message
		if len(message.ToolCalls) == 0 {
			out := make(chan adapters.LLMEvent, 2)
			content := l.finalizeToolContent(message.Content, toolResults)
			if content != "" {
				l.logger.Info("openai-compatible llm message received", "len", len(content), "text", content)
				out <- adapters.LLMEvent{Text: content}
			}
			out <- adapters.LLMEvent{Final: true}
			close(out)
			return out, nil
		}

		messages = append(messages, chatMessage{
			Role:      "assistant",
			Content:   message.Content,
			ToolCalls: message.ToolCalls,
		})
		for _, toolCall := range message.ToolCalls {
			result := l.callTool(ctx, tools, toolCall)
			toolResults = append(toolResults, ToolCallResult{
				Name:    toolCall.Function.Name,
				Content: result,
			})
			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Content:    result,
			})
		}
	}

	return nil, fmt.Errorf("openai-compatible llm exceeded max tool iterations")
}

func (l *LLM) finalizeToolContent(content string, results []ToolCallResult) string {
	for _, finalizer := range l.finalizers {
		if finalizer == nil {
			continue
		}
		content = finalizer(content, results)
	}
	return content
}

func (l *LLM) sendChatRequest(ctx context.Context, req chatCompletionRequest) (*http.Response, error) {
	body, err := json.Marshal(req)
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

	return resp, nil
}

func toolDefinitions(tools []Tool) []Tool {
	definitions := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		definitions = append(definitions, Tool{
			Type:     tool.Type,
			Function: tool.Function,
		})
	}
	return definitions
}

func (l *LLM) callTool(ctx context.Context, tools []Tool, toolCall chatToolCall) string {
	tool, ok := findTool(tools, toolCall.Function.Name)
	if !ok || tool.Handler == nil {
		return marshalToolResult(map[string]any{
			"error": fmt.Sprintf("tool %q is not available", toolCall.Function.Name),
		})
	}

	l.logger.Info("openai-compatible llm tool call started", "tool", toolCall.Function.Name, "tool_call_id", toolCall.ID)
	result, err := tool.Handler(ctx, json.RawMessage(toolCall.Function.Arguments))
	if err != nil {
		l.logger.Error("openai-compatible llm tool call failed", "tool", toolCall.Function.Name, "tool_call_id", toolCall.ID, "err", err)
		return marshalToolResult(map[string]any{"error": err.Error()})
	}

	l.logger.Info("openai-compatible llm tool call completed", "tool", toolCall.Function.Name, "tool_call_id", toolCall.ID)
	return marshalToolResult(result)
}

func findTool(tools []Tool, name string) (Tool, bool) {
	for _, tool := range tools {
		if tool.Function.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

func marshalToolResult(result any) string {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
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
	Tools       []Tool        `json:"tools,omitempty"`
	ToolChoice  string        `json:"tool_choice,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatCompletionResponse struct {
	Choices []chatResponseChoice `json:"choices"`
}

type chatResponseChoice struct {
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type chatToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function chatToolCallFunction `json:"function"`
}

type chatToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
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
