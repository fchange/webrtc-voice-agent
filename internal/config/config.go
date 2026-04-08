package config

import (
	"os"
	"strconv"
	"time"
)

type SignalConfig struct {
	Addr        string
	PublicWSURL string
	DevToken    string
	BotBaseURL  string
}

type BotConfig struct {
	Addr         string
	PublicRTCIP  string
	STUNURL      string
	IdleTimeout  time.Duration
	ControlLabel string
	SignalWSURL  string
	SignalToken  string
	VAD          VADConfig
	ASR          ASRConfig
}

type VADConfig struct {
	Provider           string
	Mode               string
	ModelScopeModelID  string
	Runtime            string
	SampleRate         int
	EndpointingSilence time.Duration
}

type ASRConfig struct {
	Provider string
	XFYUN    XFYUNASRConfig
}

type XFYUNASRConfig struct {
	WSURL         string
	Host          string
	RequestPath   string
	AppID         string
	APIKey        string
	APISecret     string
	Language      string
	Domain        string
	Accent        string
	SampleRate    int
	AudioEncoding string
	EOSMS         int
	DWA           string
}

func LoadSignalConfig() SignalConfig {
	return SignalConfig{
		Addr:        getEnv("SIGNAL_ADDR", ":8080"),
		PublicWSURL: getEnv("SIGNAL_PUBLIC_WS_URL", "ws://localhost:8080/ws"),
		DevToken:    getEnv("SIGNAL_DEV_TOKEN", "dev-token"),
		BotBaseURL:  getEnv("SIGNAL_BOT_BASE_URL", "http://localhost:8081"),
	}
}

func LoadBotConfig() BotConfig {
	return BotConfig{
		Addr:         getEnv("BOT_ADDR", ":8081"),
		PublicRTCIP:  getEnv("BOT_PUBLIC_RTC_IP", "127.0.0.1"),
		STUNURL:      getEnv("BOT_STUN_URL", "stun:stun.l.google.com:19302"),
		IdleTimeout:  getDuration("BOT_IDLE_TIMEOUT", 2*time.Minute),
		ControlLabel: getEnv("BOT_CONTROL_LABEL", "control"),
		SignalWSURL:  getEnv("BOT_SIGNAL_WS_URL", "ws://localhost:8080/ws"),
		SignalToken:  getEnv("BOT_SIGNAL_TOKEN", "dev-token"),
		VAD: VADConfig{
			Provider:           getEnv("VAD_PROVIDER", "modelscope-fsmn"),
			Mode:               getEnv("VAD_MODE", "server"),
			ModelScopeModelID:  getEnv("VAD_MODELSCOPE_MODEL_ID", "damo/speech_fsmn_vad_zh-cn-16k-common-pytorch"),
			Runtime:            getEnv("VAD_RUNTIME", "funasr"),
			SampleRate:         getInt("VAD_SAMPLE_RATE", 16000),
			EndpointingSilence: time.Duration(getInt("VAD_ENDPOINTING_SILENCE_MS", 900)) * time.Millisecond,
		},
		ASR: ASRConfig{
			Provider: getEnv("ASR_PROVIDER", "mock"),
			XFYUN: XFYUNASRConfig{
				WSURL:         getEnv("ASR_XFYUN_WS_URL", "wss://iat.xf-yun.com/v1"),
				Host:          getEnv("ASR_XFYUN_HOST", "iat.xf-yun.com"),
				RequestPath:   getEnv("ASR_XFYUN_REQUEST_PATH", "/v1"),
				AppID:         getEnv("ASR_XFYUN_APP_ID", ""),
				APIKey:        getEnv("ASR_XFYUN_API_KEY", ""),
				APISecret:     getEnv("ASR_XFYUN_API_SECRET", ""),
				Language:      getEnv("ASR_XFYUN_LANGUAGE", "zh_cn"),
				Domain:        getEnv("ASR_XFYUN_DOMAIN", "slm"),
				Accent:        getEnv("ASR_XFYUN_ACCENT", "mandarin"),
				SampleRate:    getInt("ASR_XFYUN_SAMPLE_RATE", 16000),
				AudioEncoding: getEnv("ASR_XFYUN_AUDIO_ENCODING", "raw"),
				EOSMS:         getInt("ASR_XFYUN_EOS_MS", 6000),
				DWA:           getEnv("ASR_XFYUN_DWA", "wpgs"),
			},
		},
	}
}

func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}

	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		parsed, err := time.ParseDuration(value)
		if err == nil {
			return parsed
		}
	}

	return fallback
}

func getInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}

	return fallback
}
