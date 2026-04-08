package bot

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters/mock"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/hotel"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/logging"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/observability"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

func TestHandleListRoomTypes(t *testing.T) {
	server := newTestServer()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/internal/room-types", nil)

	server.handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	var payload struct {
		RoomTypes []hotel.RoomType `json:"room_types"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.RoomTypes) != 3 {
		t.Fatalf("expected 3 room types, got %d", len(payload.RoomTypes))
	}
}

func TestHandleCreateReservationReturnsCreatedAndDeductsInventory(t *testing.T) {
	server := newTestServer()
	body := bytes.NewBufferString(`{"room_type_id":"deluxe-king","guest_name":"Ada Lovelace","phone_number":"13800138000"}`)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/reservations", body)
	request.Header.Set("Content-Type", "application/json")

	server.handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", recorder.Code)
	}

	var payload hotel.Reservation
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != hotel.ReservationStatusConfirmed {
		t.Fatalf("expected confirmed, got %s", payload.Status)
	}

	roomTypes := server.deps.Hotel.ListRoomTypes()
	if roomTypes[0].AvailableCount != 2 {
		t.Fatalf("expected deluxe-king availability 2, got %d", roomTypes[0].AvailableCount)
	}
}

func TestHandleCreateReservationReturnsConflictWhenSoldOut(t *testing.T) {
	server := newTestServer()
	server.deps.Hotel.CreateReservation(hotel.CreateReservationInput{
		RoomTypeID:  "family-suite",
		GuestName:   "First Guest",
		PhoneNumber: "10086",
	})

	body := bytes.NewBufferString(`{"room_type_id":"family-suite","guest_name":"Second Guest","phone_number":"10010"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/reservations", body)
	request.Header.Set("Content-Type", "application/json")

	server.handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", recorder.Code)
	}

	var payload hotel.Reservation
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != hotel.ReservationStatusSoldOut {
		t.Fatalf("expected sold_out, got %s", payload.Status)
	}
}

func newTestServer() *Server {
	return NewServer(
		config.BotConfig{
			IdleTimeout: time.Minute,
		},
		logging.New("bot-test"),
		Dependencies{
			Manager: session.NewManager(time.Minute),
			Metrics: observability.NewMetrics(),
			Providers: adapters.ProviderBundle{
				ASR: mock.NewASR(),
				LLM: mock.NewLLM(),
				TTS: mock.NewTTS(),
			},
			Hotel: hotel.NewStore(),
		},
	)
}
