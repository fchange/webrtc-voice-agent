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
		return llm.
			WithTools(hotelTools(store)).
			WithToolFinalizers([]openaicompat.ToolFinalizer{hotelBookingConfirmationFinalizer})
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
				Description: "在用户确认房型、入住人姓名和手机号后创建酒店预订，并在成功时扣减库存。Demo 环境允许 3 到 20 位纯数字号码；可把“5个1”理解为 11111。只有工具返回 status=confirmed 后才可以回复预订成功；invalid_input 时必须追问缺失或无效信息。",
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
							"description": "入住人手机号或 Demo 号码，例如 13800138000、111、11111；“5个1”应作为 11111 传入。",
						},
					},
					"required":             []string{"room_type_id", "guest_name", "phone_number"},
					"additionalProperties": false,
				},
			},
			ShouldUse: func(req adapters.CompletionRequest) bool {
				return hotelQueryNeedsReservation(req)
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
	return strings.ContainsAny(text, "房住订预库存还有几间姓名名字电话手机号码")
}

var phoneNumberPattern = regexp.MustCompile(`1[3-9]\d{9}`)

func hotelQueryNeedsReservation(req adapters.CompletionRequest) bool {
	text := req.Text
	if phoneNumberPattern.MatchString(text) {
		return hotelTextHasBookingIntent(text)
	}
	if hotelTextHasBookingIntent(text) && hotelTextHasGuestDetails(text) {
		return true
	}
	if !hotelTextHasGuestDetails(text) {
		return false
	}
	for _, item := range req.History {
		if item.Role == "user" && hotelTextHasBookingIntent(item.Text) {
			return true
		}
	}
	return false
}

func hotelTextHasBookingIntent(text string) bool {
	return strings.ContainsAny(text, "订预住") || strings.Contains(text, "下单")
}

func hotelTextHasGuestDetails(text string) bool {
	return strings.Contains(text, "姓名") ||
		strings.Contains(text, "名字") ||
		strings.Contains(text, "手机号") ||
		strings.Contains(text, "电话") ||
		strings.Contains(text, "号码")
}

func hotelBookingConfirmationFinalizer(content string, results []openaicompat.ToolCallResult) string {
	if !hotelTextLooksConfirmed(content) {
		return content
	}
	result, ok := latestReservationToolResult(results)
	if ok && result.Status == hotel.ReservationStatusConfirmed {
		return content
	}
	if ok && result.Message != "" {
		return "还不能确认预订：" + result.Message + "。请补充有效信息。"
	}
	return "我还没完成预订，请提供有效手机号或数字号码，我再帮您确认。"
}

func hotelTextLooksConfirmed(text string) bool {
	return strings.Contains(text, "已确认") ||
		strings.Contains(text, "预订成功") ||
		strings.Contains(text, "订好了") ||
		strings.Contains(text, "已为您预订") ||
		strings.Contains(text, "期待您光临")
}

func latestReservationToolResult(results []openaicompat.ToolCallResult) (hotel.Reservation, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		result := results[idx]
		if result.Name != "create_reservation" {
			continue
		}
		var reservation hotel.Reservation
		if err := json.Unmarshal([]byte(result.Content), &reservation); err != nil {
			return hotel.Reservation{}, false
		}
		return reservation, true
	}
	return hotel.Reservation{}, false
}
