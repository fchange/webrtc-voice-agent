package bot

import (
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

	if len(exposed) != 1 || exposed[0] != "list_room_types" {
		t.Fatalf("expected only list_room_types for availability question, got %#v", exposed)
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

	if len(exposed) != 2 {
		t.Fatalf("expected inventory and reservation tools, got %#v", exposed)
	}
	if exposed[0] != "list_room_types" || exposed[1] != "create_reservation" {
		t.Fatalf("unexpected exposed tools: %#v", exposed)
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
