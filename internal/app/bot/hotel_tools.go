package bot

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters/openaicompat"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/hotel"
)

func withHotelLLMCapabilities(provider adapters.LLMAdapter, store *hotel.Store) adapters.LLMAdapter {
	if llm, ok := provider.(*openaicompat.LLM); ok {
		return llm.WithTools(hotelTools(store))
	}

	return newHotelInventoryLLM(provider, store)
}

func hotelTools(store *hotel.Store) []openaicompat.Tool {
	return []openaicompat.Tool{
		{
			Type: "function",
			Function: openaicompat.ToolFunction{
				Name:        "list_room_types",
				Description: "查询酒店全部房型、价格、入住人数和实时剩余库存。",
				Parameters: map[string]any{
					"type":                 "object",
					"properties":           map[string]any{},
					"additionalProperties": false,
				},
			},
			ShouldUse: func(req adapters.CompletionRequest) bool {
				return hotelQueryNeedsInventory(req.Text)
			},
			Handler: func(context.Context, json.RawMessage) (any, error) {
				return map[string]any{
					"room_types": store.ListRoomTypes(),
				}, nil
			},
		},
		{
			Type: "function",
			Function: openaicompat.ToolFunction{
				Name:        "create_reservation",
				Description: "在用户确认房型、入住人姓名和手机号后创建酒店预订，并在成功时扣减库存。",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"room_type_id": map[string]any{
							"type":        "string",
							"description": "房型 ID，例如 deluxe-king、garden-twin、family-suite。",
						},
						"guest_name": map[string]any{
							"type":        "string",
							"description": "入住人姓名。",
						},
						"phone_number": map[string]any{
							"type":        "string",
							"description": "入住人手机号。",
						},
					},
					"required":             []string{"room_type_id", "guest_name", "phone_number"},
					"additionalProperties": false,
				},
			},
			ShouldUse: func(req adapters.CompletionRequest) bool {
				return hotelQueryNeedsReservation(req.Text)
			},
			Handler: func(_ context.Context, arguments json.RawMessage) (any, error) {
				var input hotel.CreateReservationInput
				if len(arguments) > 0 {
					if err := json.Unmarshal(arguments, &input); err != nil {
						return nil, err
					}
				}
				return store.CreateReservation(input), nil
			},
		},
	}
}

func hotelQueryNeedsInventory(text string) bool {
	return strings.ContainsAny(text, "房住订预库存还有几间")
}

var phoneNumberPattern = regexp.MustCompile(`1[3-9]\d{9}`)

func hotelQueryNeedsReservation(text string) bool {
	if !phoneNumberPattern.MatchString(text) {
		return false
	}
	return strings.Contains(text, "订") || strings.Contains(text, "预订") || strings.Contains(text, "下单")
}
