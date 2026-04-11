package hotel

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ReservationStatus string

const (
	ReservationStatusConfirmed    ReservationStatus = "confirmed"
	ReservationStatusSoldOut      ReservationStatus = "sold_out"
	ReservationStatusInvalidInput ReservationStatus = "invalid_input"
	ReservationStatusFailed       ReservationStatus = "failed"
)

type RoomType struct {
	ID             string `json:"room_type_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	PriceLabel     string `json:"price_label"`
	Capacity       int    `json:"capacity"`
	AvailableCount int    `json:"available_count"`
}

type Reservation struct {
	ID                  string            `json:"reservation_id"`
	Status              ReservationStatus `json:"status"`
	Message             string            `json:"message"`
	RoomTypeID          string            `json:"room_type_id"`
	RoomTypeName        string            `json:"room_type_name,omitempty"`
	GuestName           string            `json:"guest_name"`
	PhoneNumber         string            `json:"phone_number"`
	AvailableCountAfter int               `json:"available_count_after"`
	CreatedAt           time.Time         `json:"created_at"`
}

type CreateReservationInput struct {
	RoomTypeID  string `json:"room_type_id"`
	GuestName   string `json:"guest_name"`
	PhoneNumber string `json:"phone_number"`
}

type Store struct {
	mu           sync.RWMutex
	roomOrder    []string
	rooms        map[string]*RoomType
	reservations []Reservation
	nextID       int64
}

var demoPhonePattern = regexp.MustCompile(`^\d{3,20}$`)
var repeatedDigitPhonePattern = regexp.MustCompile(`^([0-9]+)个([0-9])$`)

func NewStore() *Store {
	seed := []RoomType{
		{
			ID:             "deluxe-king",
			Name:           "豪华大床房",
			Description:    "高楼层城景房，含双早，适合情侣或商务出行。",
			PriceLabel:     "688 元 / 晚",
			Capacity:       2,
			AvailableCount: 3,
		},
		{
			ID:             "garden-twin",
			Name:           "花园双床房",
			Description:    "面向庭院的双床房，适合朋友同行或亲子入住。",
			PriceLabel:     "520 元 / 晚",
			Capacity:       2,
			AvailableCount: 2,
		},
		{
			ID:             "family-suite",
			Name:           "家庭套房",
			Description:    "带休闲区的大套房，含一张大床和沙发床。",
			PriceLabel:     "980 元 / 晚",
			Capacity:       4,
			AvailableCount: 1,
		},
	}

	store := &Store{
		roomOrder: make([]string, 0, len(seed)),
		rooms:     make(map[string]*RoomType, len(seed)),
		nextID:    1,
	}

	for _, room := range seed {
		roomCopy := room
		store.roomOrder = append(store.roomOrder, room.ID)
		store.rooms[room.ID] = &roomCopy
	}

	return store
}

func (s *Store) ListRoomTypes() []RoomType {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rooms := make([]RoomType, 0, len(s.roomOrder))
	for _, roomID := range s.roomOrder {
		if room, ok := s.rooms[roomID]; ok {
			rooms = append(rooms, *room)
		}
	}

	return rooms
}

func (s *Store) ListReservations(limit int) []Reservation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.reservations) {
		limit = len(s.reservations)
	}

	items := make([]Reservation, 0, limit)
	for idx := len(s.reservations) - 1; idx >= 0 && len(items) < limit; idx-- {
		items = append(items, s.reservations[idx])
	}

	return items
}

func (s *Store) CreateReservation(input CreateReservationInput) Reservation {
	s.mu.Lock()
	defer s.mu.Unlock()

	guestName := strings.TrimSpace(input.GuestName)
	phoneNumber := normalizeDemoPhoneNumber(input.PhoneNumber)
	roomTypeID := strings.TrimSpace(input.RoomTypeID)
	reservation := Reservation{
		ID:          s.nextReservationIDLocked(),
		RoomTypeID:  roomTypeID,
		GuestName:   guestName,
		PhoneNumber: phoneNumber,
		CreatedAt:   time.Now().UTC(),
	}

	if roomTypeID == "" || guestName == "" || phoneNumber == "" {
		reservation.Status = ReservationStatusInvalidInput
		reservation.Message = "房型、入住人姓名和手机号不能为空"
		s.reservations = append(s.reservations, reservation)
		return reservation
	}
	if !demoPhonePattern.MatchString(phoneNumber) {
		reservation.Status = ReservationStatusInvalidInput
		reservation.Message = "手机号需为3到20位数字"
		s.reservations = append(s.reservations, reservation)
		return reservation
	}

	room, ok := s.rooms[roomTypeID]
	if !ok {
		reservation.Status = ReservationStatusInvalidInput
		reservation.Message = "未找到对应房型"
		s.reservations = append(s.reservations, reservation)
		return reservation
	}

	reservation.RoomTypeName = room.Name
	reservation.AvailableCountAfter = room.AvailableCount
	if room.AvailableCount < 1 {
		reservation.Status = ReservationStatusSoldOut
		reservation.Message = fmt.Sprintf("%s当前已售罄", room.Name)
		s.reservations = append(s.reservations, reservation)
		return reservation
	}

	room.AvailableCount--
	reservation.Status = ReservationStatusConfirmed
	reservation.AvailableCountAfter = room.AvailableCount
	reservation.Message = fmt.Sprintf("%s已为%s预订成功", room.Name, guestName)
	s.reservations = append(s.reservations, reservation)
	return reservation
}

func (s *Store) nextReservationIDLocked() string {
	next := s.nextID
	s.nextID++
	return fmt.Sprintf("res_%06d", next)
}

func normalizeDemoPhoneNumber(phoneNumber string) string {
	normalized := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(phoneNumber))
	matches := repeatedDigitPhonePattern.FindStringSubmatch(normalized)
	if len(matches) != 3 {
		return normalized
	}

	count, err := strconv.Atoi(matches[1])
	if err != nil || count < 1 || count > 20 {
		return normalized
	}
	return strings.Repeat(matches[2], count)
}
