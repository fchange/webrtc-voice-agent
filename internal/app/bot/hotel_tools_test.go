package bot

import "testing"

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
