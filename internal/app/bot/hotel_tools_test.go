package bot

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters/openaicompat"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/hotel"
)

func TestHotelQueryNeedsReservationRequiresBookingDetails(t *testing.T) {
	if hotelQueryNeedsReservation(adapters.CompletionRequest{Text: "总统套房还有几间"}) {
		t.Fatal("availability question should not expose reservation tool")
	}
	if hotelQueryNeedsReservation(adapters.CompletionRequest{Text: "我要订家庭套房"}) {
		t.Fatal("booking intent without details should not expose reservation tool")
	}
	if !hotelQueryNeedsReservation(adapters.CompletionRequest{Text: "我要订家庭套房，张三，手机号 13800138000"}) {
		t.Fatal("booking intent with phone should expose reservation tool")
	}
	if !hotelQueryNeedsReservation(adapters.CompletionRequest{
		Text: "今晚入住，我的名字叫方程，号码是5个1",
		History: []adapters.ConversationMessage{
			{Role: "user", Text: "我要订家庭套房"},
		},
	}) {
		t.Fatal("booking details after prior booking intent should expose reservation tool")
	}
}

func TestHotelToolsExposeOnlyInventoryForAvailabilityQuestion(t *testing.T) {
	tools := hotelTools(hotel.NewStore())
	var exposed []string
	req := adapters.CompletionRequest{Text: "总统套房还有几间"}

	for _, tool := range tools {
		if tool.ShouldUse == nil || tool.ShouldUse(req) {
			exposed = append(exposed, tool.Function.Name)
		}
	}

	if len(exposed) != 2 {
		t.Fatalf("expected list_room_types and end_call for availability question, got %#v", exposed)
	}
	if exposed[0] != "list_room_types" || exposed[1] != "end_call" {
		t.Fatalf("unexpected exposed tools: %#v", exposed)
	}
}

func TestHotelToolsExposeReservationOnlyWhenUserProvidesBookingDetails(t *testing.T) {
	tools := hotelTools(hotel.NewStore())
	var exposed []string
	req := adapters.CompletionRequest{Text: "我要订家庭套房，张三，手机号 13800138000"}

	for _, tool := range tools {
		if tool.ShouldUse == nil || tool.ShouldUse(req) {
			exposed = append(exposed, tool.Function.Name)
		}
	}

	if len(exposed) != 3 {
		t.Fatalf("expected inventory, reservation, and end_call tools, got %#v", exposed)
	}
	if exposed[0] != "list_room_types" || exposed[1] != "create_reservation" || exposed[2] != "end_call" {
		t.Fatalf("unexpected exposed tools: %#v", exposed)
	}
}

func TestHotelToolsExposeEndCallForFarewell(t *testing.T) {
	tools := hotelTools(hotel.NewStore())
	var exposed []string
	req := adapters.CompletionRequest{
		Text: "谢谢，再见",
		History: []adapters.ConversationMessage{
			{Role: "assistant", Text: "花园双床房已为您预订成功，确认号 res_000001。"},
		},
	}

	for _, tool := range tools {
		if tool.ShouldUse == nil || tool.ShouldUse(req) {
			exposed = append(exposed, tool.Function.Name)
		}
	}

	if len(exposed) != 1 || exposed[0] != "end_call" {
		t.Fatalf("expected only end_call for farewell request, got %#v", exposed)
	}
}

func TestHotelBookingConfirmationFinalizerBlocksConfirmationWithoutConfirmedToolResult(t *testing.T) {
	got := hotelBookingConfirmationFinalizer("已确认您的预订，期待您光临！", nil)

	if got == "已确认您的预订，期待您光临！" {
		t.Fatal("expected confirmation to be blocked without reservation tool result")
	}
}

func TestHotelBookingConfirmationFinalizerPreservesConfirmedReservation(t *testing.T) {
	result := `{"status":"confirmed","message":"家庭套房已为方程预订成功"}`
	got := hotelBookingConfirmationFinalizer("已确认您的预订，期待您光临！", []openaicompat.ToolCallResult{
		{Name: "create_reservation", Content: result},
	})

	if got != "已确认您的预订，期待您光临！" {
		t.Fatalf("expected confirmed reservation response to pass through, got %q", got)
	}
}

func TestEndCallToolMarksPendingDirective(t *testing.T) {
	tools := hotelTools(hotel.NewStore())
	var endCall openaicompat.Tool
	found := false
	for _, tool := range tools {
		if tool.Function.Name == "end_call" {
			endCall = tool
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected end_call tool to exist")
	}

	directives := newTurnDirectives()
	result, err := endCall.Handler(
		withTurnDirectives(context.Background(), directives),
		json.RawMessage(`{"reason":"reservation_confirmed"}`),
	)
	if err != nil {
		t.Fatalf("end_call handler failed: %v", err)
	}

	intent, ok := directives.EndCall()
	if !ok {
		t.Fatal("expected end_call to set pending directive")
	}
	if intent.Reason != "reservation_confirmed" {
		t.Fatalf("unexpected directive reason: %+v", intent)
	}
	if intent.Message == "" {
		t.Fatalf("expected directive message to be populated: %+v", intent)
	}

	payload, ok := result.(map[string]any)
	if !ok || payload["status"] != "scheduled" {
		t.Fatalf("unexpected end_call result: %#v", result)
	}
}
