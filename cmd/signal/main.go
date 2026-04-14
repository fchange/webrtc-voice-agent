package main

import (
	"log"

	"github.com/fchange/webrtc-voice-agent/internal/app/signal"
	"github.com/fchange/webrtc-voice-agent/internal/config"
	"github.com/fchange/webrtc-voice-agent/internal/logging"
)

func main() {
	cfg := config.LoadSignalConfig()
	logger := logging.New("signal")

	if err := signal.NewServer(cfg, logger).Run(); err != nil {
		log.Fatal(err)
	}
}
