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
	LLM          LLMConfig
	TTS          TTSConfig
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

type LLMConfig struct {
	Provider     string
	Segmenter    LLMSegmenterConfig
	OpenAICompat OpenAICompatibleLLMConfig
}

type LLMSegmenterConfig struct {
	Mode        string
	Punctuation string
}

type OpenAICompatibleLLMConfig struct {
	BaseURL         string
	APIKey          string
	Model           string
	SystemPrompt    string
	MaxTokens       int
	Temperature     float64
	TopP            float64
	TopK            int
	FailoverEnabled bool
	Timeout         time.Duration
}

type TTSConfig struct {
	Provider string
	XFYUN    XFYUNTTSConfig
}

type XFYUNTTSConfig struct {
	WSURL         string
	Host          string
	RequestPath   string
	AppID         string
	APIKey        string
	APISecret     string
	Voice         string
	TextEncoding  string
	AudioEncoding string
	AudioFormat   string
	PCMEndian     string
	SampleRate    int
	Speed         int
	Volume        int
	Pitch         int
	Background    int
	DebugDumpDir  string
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
		LLM: LLMConfig{
			Provider: getEnv("LLM_PROVIDER", "mock"),
			Segmenter: LLMSegmenterConfig{
				Mode:        getEnv("LLM_SEGMENTER_MODE", "punctuation_boundary"),
				Punctuation: getEnv("LLM_SEGMENTER_PUNCTUATION", "。！？；!?;"),
			},
			OpenAICompat: OpenAICompatibleLLMConfig{
				BaseURL:         getEnv("LLM_OPENAI_COMPAT_BASE_URL", "https://ai.gitee.com/v1/chat/completions"),
				APIKey:          getEnv("LLM_OPENAI_COMPAT_API_KEY", ""),
				Model:           getEnv("LLM_OPENAI_COMPAT_MODEL", "Qwen2-7B-Instruct"),
				SystemPrompt:    getEnv("LLM_OPENAI_COMPAT_SYSTEM_PROMPT", "You are a concise and helpful Chinese voice assistant."),
				MaxTokens:       getInt("LLM_OPENAI_COMPAT_MAX_TOKENS", 512),
				Temperature:     getFloat("LLM_OPENAI_COMPAT_TEMPERATURE", 0.7),
				TopP:            getFloat("LLM_OPENAI_COMPAT_TOP_P", 0.7),
				TopK:            getInt("LLM_OPENAI_COMPAT_TOP_K", 50),
				FailoverEnabled: getBool("LLM_OPENAI_COMPAT_FAILOVER_ENABLED", true),
				Timeout:         getDuration("LLM_OPENAI_COMPAT_TIMEOUT", 45*time.Second),
			},
		},
		TTS: TTSConfig{
			Provider: getEnv("TTS_PROVIDER", "mock"),
			XFYUN: XFYUNTTSConfig{
				WSURL:         getEnv("TTS_XFYUN_WS_URL", "wss://tts-api.xfyun.cn/v2/tts"),
				Host:          getEnv("TTS_XFYUN_HOST", "tts-api.xfyun.cn"),
				RequestPath:   getEnv("TTS_XFYUN_REQUEST_PATH", "/v2/tts"),
				AppID:         getEnv("TTS_XFYUN_APP_ID", ""),
				APIKey:        getEnv("TTS_XFYUN_API_KEY", ""),
				APISecret:     getEnv("TTS_XFYUN_API_SECRET", ""),
				Voice:         getEnv("TTS_XFYUN_VOICE", "xiaoyan"),
				TextEncoding:  getEnv("TTS_XFYUN_TEXT_ENCODING", "utf8"),
				AudioEncoding: getEnv("TTS_XFYUN_AUDIO_ENCODING", "raw"),
				AudioFormat:   getEnv("TTS_XFYUN_AUDIO_FORMAT", "audio/L16;rate=16000"),
				PCMEndian:     getEnv("TTS_XFYUN_PCM_ENDIAN", "little"),
				SampleRate:    getInt("TTS_XFYUN_SAMPLE_RATE", 16000),
				Speed:         getInt("TTS_XFYUN_SPEED", 50),
				Volume:        getInt("TTS_XFYUN_VOLUME", 50),
				Pitch:         getInt("TTS_XFYUN_PITCH", 50),
				Background:    getInt("TTS_XFYUN_BACKGROUND", 0),
				DebugDumpDir:  getEnv("TTS_DEBUG_DUMP_DIR", ""),
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

func getFloat(key string, fallback float64) float64 {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		parsed, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return parsed
		}
	}

	return fallback
}

func getBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}

	return fallback
}
