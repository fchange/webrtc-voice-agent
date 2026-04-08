package xfyun

import (
	"net/url"
	"testing"
	"time"
)

func TestBuildAuthURL(t *testing.T) {
	got, err := BuildAuthURL(
		"wss://iat.xf-yun.com/v1",
		"iat.xf-yun.com",
		"/v1",
		"test-key",
		"test-secret",
		time.Date(2024, 5, 14, 8, 46, 48, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("build auth url: %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	query := parsed.Query()
	if query.Get("host") != "iat.xf-yun.com" {
		t.Fatalf("expected host query param, got %q", query.Get("host"))
	}
	if query.Get("date") == "" {
		t.Fatal("expected date query param")
	}
	if query.Get("authorization") == "" {
		t.Fatal("expected authorization query param")
	}
}
