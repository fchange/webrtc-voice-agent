package openaicompat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
)

func TestReadStreamHandlesSSEChunks(t *testing.T) {
	provider := NewLLM(configForTest(), nil)
	body := strings.NewReader(strings.Join([]string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"你好\"},\"finish_reason\":\"\"}]}",
		"",
		"data: {\"choices\":[{\"delta\":{\"content\":\"。\"},\"finish_reason\":\"\"}]}",
		"",
		"data: [DONE]",
		"",
	}, "\n"))

	out := make(chan adapters.LLMEvent, 8)
	provider.readStream(io.NopCloser(body), out)

	var events []adapters.LLMEvent
	for event := range out {
		events = append(events, event)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Text != "你好" || events[0].Final {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].Text != "。" || events[1].Final {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
	if !events[2].Final {
		t.Fatalf("expected final event, got %+v", events[2])
	}
}

func TestCompleteRunsToolCallLoop(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			if len(req.Tools) != 1 || req.Tools[0].Function.Name != "list_room_types" {
				t.Fatalf("expected list_room_types tool, got %+v", req.Tools)
			}
			_ = json.NewEncoder(w).Encode(chatCompletionResponse{
				Choices: []chatResponseChoice{
					{
						FinishReason: "tool_calls",
						Message: chatMessage{
							Role: "assistant",
							ToolCalls: []chatToolCall{
								{
									ID:   "call_1",
									Type: "function",
									Function: chatToolCallFunction{
										Name:      "list_room_types",
										Arguments: `{}`,
									},
								},
							},
						},
					},
				},
			})
		case 2:
			if len(req.Messages) < 4 {
				t.Fatalf("expected tool result in follow-up messages, got %+v", req.Messages)
			}
			toolMessage := req.Messages[len(req.Messages)-1]
			if toolMessage.Role != "tool" || toolMessage.ToolCallID != "call_1" {
				t.Fatalf("expected tool result message, got %+v", toolMessage)
			}
			if !strings.Contains(toolMessage.Content, "家庭套房") {
				t.Fatalf("expected tool content to include handler result, got %q", toolMessage.Content)
			}
			_ = json.NewEncoder(w).Encode(chatCompletionResponse{
				Choices: []chatResponseChoice{
					{
						FinishReason: "stop",
						Message: chatMessage{
							Role:    "assistant",
							Content: "家庭套房还有 1 间。",
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	provider := NewLLM(configForTest(), nil).WithTools([]Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name: "list_room_types",
				Parameters: map[string]any{
					"type": "object",
				},
			},
			Handler: func(context.Context, json.RawMessage) (any, error) {
				return map[string]any{
					"room_types": []map[string]any{
						{"name": "家庭套房", "available_count": 1},
					},
				}, nil
			},
		},
	})
	provider.cfg.BaseURL = server.URL

	events, err := provider.Complete(t.Context(), adapters.CompletionRequest{Text: "家庭套房还有吗"})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	var got strings.Builder
	final := false
	for event := range events {
		got.WriteString(event.Text)
		final = final || event.Final
	}
	if got.String() != "家庭套房还有 1 间。" || !final {
		t.Fatalf("unexpected events text=%q final=%v", got.String(), final)
	}
	if requestCount != 2 {
		t.Fatalf("expected two requests, got %d", requestCount)
	}
}

func TestCompleteFiltersToolsPerRequest(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Tools) != 1 || req.Tools[0].Function.Name != "list_room_types" {
			t.Fatalf("availability request should expose only list_room_types, got %+v", req.Tools)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatCompletionResponse{
			Choices: []chatResponseChoice{
				{
					FinishReason: "stop",
					Message: chatMessage{
						Role:    "assistant",
						Content: "没有总统套房。",
					},
				},
			},
		})
	}))
	defer server.Close()

	provider := NewLLM(configForTest(), nil).WithTools([]Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name: "list_room_types",
			},
			Handler: func(context.Context, json.RawMessage) (any, error) {
				return map[string]any{"room_types": []any{}}, nil
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "create_reservation",
			},
			ShouldUse: func(adapters.CompletionRequest) bool {
				return false
			},
			Handler: func(context.Context, json.RawMessage) (any, error) {
				t.Fatal("reservation tool should not be callable for availability request")
				return nil, nil
			},
		},
	})
	provider.cfg.BaseURL = server.URL

	events, err := provider.Complete(t.Context(), adapters.CompletionRequest{Text: "总统套房还有几间"})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	for range events {
	}
	if requestCount != 1 {
		t.Fatalf("expected one request, got %d", requestCount)
	}
}

func configForTest() config.OpenAICompatibleLLMConfig {
	return config.OpenAICompatibleLLMConfig{
		BaseURL:   "https://example.com/v1/chat/completions",
		APIKey:    "test",
		Model:     "Qwen2-7B-Instruct",
		Timeout:   0,
		MaxTokens: 128,
	}
}
