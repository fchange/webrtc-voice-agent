package main

import (
	"log"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters/mock"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters/xfyun"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/app/bot"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/config"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/logging"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/observability"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/session"
)

func main() {
	cfg := config.LoadBotConfig()
	logger := logging.New("bot")
	asrProvider := selectASRProvider(cfg)

	deps := bot.Dependencies{
		Manager: session.NewManager(cfg.IdleTimeout),
		Metrics: observability.NewMetrics(),
		Providers: mock.ProviderBundle{
			ASR: asrProvider,
			LLM: mock.NewLLM(),
			TTS: mock.NewTTS(),
		},
	}

	if err := bot.NewServer(cfg, logger, deps).Run(); err != nil {
		log.Fatal(err)
	}
}

func selectASRProvider(cfg config.BotConfig) adapters.ASRAdapter {
	if cfg.ASR.Provider == "xfyun-spark-iat" {
		provider := xfyun.NewASR(cfg.ASR.XFYUN)
		if provider.Ready() {
			return provider
		}
	}
	return mock.NewASR()
}
