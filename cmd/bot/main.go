package main

import (
	"log"

	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters/mock"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters/openaicompat"
	"github.com/webrtc-voice-bot/webrtc-voice-bot/internal/adapters/volc"
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
	llmProvider := selectLLMProvider(cfg)
	ttsProvider := selectTTSProvider(cfg)

	deps := bot.Dependencies{
		Manager: session.NewManager(cfg.IdleTimeout),
		Metrics: observability.NewMetrics(),
		Providers: adapters.ProviderBundle{
			ASR: asrProvider,
			LLM: llmProvider,
			TTS: ttsProvider,
		},
	}

	if err := bot.NewServer(cfg, logger, deps).Run(); err != nil {
		log.Fatal(err)
	}
}

func selectASRProvider(cfg config.BotConfig) adapters.ASRAdapter {
	if cfg.ASR.Provider == "xfyun-spark-iat" {
		provider := xfyun.NewASR(cfg.ASR.XFYUN, logging.New("xfyun-asr"))
		if provider.Ready() {
			return provider
		}
	}
	if cfg.ASR.Provider == "volc-doubao-asr" {
		provider := volc.NewASR(cfg.ASR.VOLC, logging.New("volc-asr"))
		if provider.Ready() {
			return provider
		}
	}
	return mock.NewASR()
}

func selectLLMProvider(cfg config.BotConfig) adapters.LLMAdapter {
	if cfg.LLM.Provider == "openai-compatible-chat-completions" {
		provider := openaicompat.NewLLM(cfg.LLM.OpenAICompat, logging.New("openai-compatible-llm"))
		if provider.Ready() {
			return provider
		}
	}
	return mock.NewLLM()
}

func selectTTSProvider(cfg config.BotConfig) adapters.TTSAdapter {
	if cfg.TTS.Provider == "xfyun-tts" {
		provider := xfyun.NewTTS(cfg.TTS.XFYUN, logging.New("xfyun-tts"))
		if provider.Ready() {
			return provider
		}
	}
	if cfg.TTS.Provider == "volc-doubao-tts" {
		provider := volc.NewTTS(cfg.TTS.VOLC, logging.New("volc-tts"))
		if provider.Ready() {
			return provider
		}
	}
	return mock.NewTTS()
}
