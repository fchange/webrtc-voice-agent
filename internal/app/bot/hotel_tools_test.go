package bot

import (
	"testing"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/hotel"
)

func TestHotelQueryNeedsReservationRequiresPhoneAndBookingIntent(t *testing.T) {
	if hotelQueryNeedsReservation("总统套房还有几间") {
		t.Fatal("availability question should not expose reservation tool")
	}
	if hotelQueryNeedsReservation("我要订家庭套房") {
		t.Fatal("booking intent without phone should not expose reservation tool")
	}
	if !hotelQueryNeedsReservation("我要订家庭套房，张三，手机号 13800138000") {
		t.Fatal("booking intent with phone should expose reservation tool")
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
