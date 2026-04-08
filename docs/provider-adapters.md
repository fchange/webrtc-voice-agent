# Provider Adapters

## Goal

ASR / LLM / TTS 必须通过统一 adapter 接口接入，避免把 SessionTask 与某个供应商 SDK 绑死。

## Current Provider Choice

- VAD: ModelScope `damo/speech_fsmn_vad_zh-cn-16k-common-pytorch`
- ASR: 讯飞 `spark_zh_iat` WebSocket API
- LLM: OpenAI-compatible `chat/completions` 流式接口
- TTS: 讯飞在线语音合成 WebSocket API

选择原因：

- FSMN VAD 更适合低时延 turn 切分，不把 VAD 和 ASR 绑死
- 讯飞实时听写 WebSocket 接口适合流式语音上行
- OpenAI-compatible `chat/completions` 接口适合快速接入流式 token 输出
- 讯飞在线语音合成适合按句段触发的流式合成
- 两者都适合当前中文实时语音对话 MVP

## Current Code Status

- 讯飞 ASR adapter 已实现：
  - WebSocket 鉴权 URL 生成
  - 请求 / 响应 schema
  - base64 结果解码
  - `wpgs` 动态修正文本累积
- bot 启动时可根据 `ASR_PROVIDER=xfyun-spark-iat` 选择该 adapter
- bot 已建立 `Opus -> PCM -> 16k mono -> XFYUN WebSocket ASR` 的接线骨架
- 流式 transcript 已可通过 DataChannel `asr.partial / asr.final` 回推客户端
- bot 已建立 `ASR final -> LLM stream -> punctuation_boundary segmenter -> TTS stream` 的回复侧骨架
- 当前 TTS 已真实调用 provider，并已接到 WebRTC 下行音轨
- 当前 Opus decoder 已切到 `github.com/godeps/opus` 的 WASM libopus 后端
- 这比之前的 SILK-only 路线更接近浏览器真实 WebRTC Opus 兼容面
- 下一步重点不是再换 decoder，而是做浏览器实流验证、错误分型和必要的 fallback 策略

## Required Interfaces

- `ASRAdapter`
- `LLMAdapter`
- `TTSAdapter`

## Rules

- adapter 只关心 provider 协议，不关心 session 状态机
- adapter 输出统一事件或 chunk，不直接改 SessionTask 状态
- provider timeout、cancel、error 必须显式返回
- 先提供 mock adapter，方便本地开发和协议调试
- 流式 ASR 不应把每个不稳定 partial 直接推进 LLM
- 流式 TTS 必须支持 cancel，并在 interrupt 时优先停止

## Phase Strategy

- Phase 0: interface + mock
- Phase 1: demo provider
- Phase 2: production-grade provider with timeout / retry / observability

## Streaming Adapter Guidance

- Decoder:
  - 位于 WebRTC ingress 与 VAD / ASR 之间
  - 负责把 codec-aware encoded packet 转成统一 PCM frame
  - 当前已切到 WASM libopus 后端，并按 track 独立创建 decoder，避免跨会话共享状态
- ASR:
  - 输入为 decoder 后的持续音频流
  - 输出区分 partial / stable / final
  - 当前目标 provider 为讯飞 `spark_zh_iat`
- LLM:
  - 优先消费 stable / final transcript
  - 输出支持 token stream
  - 当前通过 `LLM_SEGMENTER_MODE=punctuation_boundary` 在 `。！？；!?;` 等标点处分段
- TTS:
  - 输入支持增量文本或句段
  - 输出为可取消的音频 chunk 流
  - 当前每个句段单独发起一次 provider synthesis

## Config Notes

- VAD 配置使用 `VAD_*` 前缀
- 讯飞实时听写配置使用 `ASR_XFYUN_*` 前缀
- OpenAI-compatible LLM 配置使用 `LLM_OPENAI_COMPAT_*` 前缀
- 句段切分配置使用 `LLM_SEGMENTER_*` 前缀
- 讯飞 TTS 配置使用 `TTS_XFYUN_*` 前缀
- `TTS_XFYUN_PCM_ENDIAN` 用于声明 provider 返回的 `audio/L16` 字节序，当前默认按 `little` 处理
- `TTS_DEBUG_DUMP_DIR` 可打开 TTS 调试导出，按 segment 落 `raw` / `wav` / `txt`
- 调试导出还会额外生成 `be_16k / le_16k / be_8k / le_8k` 四个 WAV 变体，便于快速判断真实 sample rate 与 endian
- 真实密钥只放 `.env.local` 或部署环境，不要写进 tracked 示例文件
