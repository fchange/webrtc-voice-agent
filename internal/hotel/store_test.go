package hotel

import "testing"

func TestCreateReservationDeductsInventory(t *testing.T) {
	store := NewStore()

	result := store.CreateReservation(CreateReservationInput{
		RoomTypeID:  "deluxe-king",
		GuestName:   "Ada Lovelace",
		PhoneNumber: "13800138000",
	})

	if result.Status != ReservationStatusConfirmed {
		t.Fatalf("expected confirmed, got %s", result.Status)
	}

	rooms := store.ListRoomTypes()
	if rooms[0].AvailableCount != 2 {
		t.Fatalf("expected deluxe-king availability to be 2, got %d", rooms[0].AvailableCount)
	}
}

func TestCreateReservationReturnsSoldOutWithoutFurtherDeduction(t *testing.T) {
	store := NewStore()

	store.CreateReservation(CreateReservationInput{
		RoomTypeID:  "family-suite",
		GuestName:   "First Guest",
		PhoneNumber: "13800138000",
	})
	soldOut := store.CreateReservation(CreateReservationInput{
		RoomTypeID:  "family-suite",
		GuestName:   "Second Guest",
		PhoneNumber: "13900139000",
	})

	if soldOut.Status != ReservationStatusSoldOut {
		t.Fatalf("expected sold_out, got %s", soldOut.Status)
	}

	rooms := store.ListRoomTypes()
	if rooms[2].AvailableCount != 0 {
		t.Fatalf("expected family-suite availability to remain 0, got %d", rooms[2].AvailableCount)
	}
}

func TestListReservationsReturnsNewestFirst(t *testing.T) {
	store := NewStore()

	store.CreateReservation(CreateReservationInput{
		RoomTypeID:  "deluxe-king",
		GuestName:   "First Guest",
		PhoneNumber: "13800138000",
	})
	second := store.CreateReservation(CreateReservationInput{
		RoomTypeID:  "garden-twin",
		GuestName:   "Second Guest",
		PhoneNumber: "13900139000",
	})

	items := store.ListReservations(10)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != second.ID {
		t.Fatalf("expected newest reservation %s, got %s", second.ID, items[0].ID)
	}
}

func TestCreateReservationAcceptsDemoPhoneNumber(t *testing.T) {
	store := NewStore()

	result := store.CreateReservation(CreateReservationInput{
		RoomTypeID:  "family-suite",
		GuestName:   "Fang Cheng",
		PhoneNumber: "11111",
	})

	if result.Status != ReservationStatusConfirmed {
		t.Fatalf("expected confirmed, got %s", result.Status)
	}

	rooms := store.ListRoomTypes()
	if rooms[2].AvailableCount != 0 {
		t.Fatalf("expected family-suite availability to be 0, got %d", rooms[2].AvailableCount)
	}
}

func TestCreateReservationNormalizesRepeatedDigitDemoPhoneNumber(t *testing.T) {
	store := NewStore()

	result := store.CreateReservation(CreateReservationInput{
		RoomTypeID:  "deluxe-king",
		GuestName:   "Fang Cheng",
		PhoneNumber: "5个1",
	})

	if result.Status != ReservationStatusConfirmed {
		t.Fatalf("expected confirmed, got %s", result.Status)
	}
	if result.PhoneNumber != "11111" {
		t.Fatalf("expected normalized phone number 11111, got %q", result.PhoneNumber)
	}
}

func TestCreateReservationRejectsInvalidPhoneNumber(t *testing.T) {
	store := NewStore()

	result := store.CreateReservation(CreateReservationInput{
		RoomTypeID:  "family-suite",
		GuestName:   "Fang Cheng",
		PhoneNumber: "abc",
	})

	if result.Status != ReservationStatusInvalidInput {
		t.Fatalf("expected invalid_input, got %s", result.Status)
	}

	rooms := store.ListRoomTypes()
	if rooms[2].AvailableCount != 1 {
		t.Fatalf("expected family-suite availability to remain 1, got %d", rooms[2].AvailableCount)
	}
}
