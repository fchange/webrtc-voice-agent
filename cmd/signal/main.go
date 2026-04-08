package main

import (
	"log"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/app/signal"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/logging"
)

func main() {
	cfg := config.LoadSignalConfig()
	logger := logging.New("signal")

	if err := signal.NewServer(cfg, logger).Run(); err != nil {
		log.Fatal(err)
	}
}
