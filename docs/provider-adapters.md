# Provider Adapters

## Goal

ASR / LLM / TTS 必须通过统一 adapter 接口接入，避免把 SessionTask 与某个供应商 SDK 绑死。

## Current Provider Choice

- VAD: ModelScope `damo/speech_fsmn_vad_zh-cn-16k-common-pytorch`
- ASR: 讯飞 `spark_zh_iat` WebSocket API
- LLM: OpenAI-compatible `chat/completions` 流式接口（可直连火山 Ark）
- TTS: 讯飞在线语音合成 WebSocket API

选择原因：

- FSMN VAD 适合低时延 turn 切分，与 ASR 解耦
- 讯飞实时听写 WebSocket 接口适配流式语音上行
- OpenAI-compatible `chat/completions` 接口可快速接入流式 token 输出（火山 Ark 兼容该格式）
- 讯飞在线语音合成支持按句段触发的流式合成
- 上述组合覆盖当前中文实时语音对话 MVP

## Volc/Doubao Providers

为支持国内可用的 ASR/TTS/LLM，补充火山（豆包）provider：

- ASR: `ASR_PROVIDER=volc-doubao-asr`
  - 基于火山「大模型流式语音识别」WebSocket（二进制协议）
  - 默认接入流式输入模式 `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel_nostream`
  - 该模式在音频超过 15s 或发送最后一个负包后返回结果，稳定性高于双向流式模式
  - 通过 Header 传 `X-Api-App-Key / X-Api-Access-Key / X-Api-Resource-Id`
  - `ASR_VOLC_RESOURCE_ID` 使用官方文档中的 `volc.bigasr.sauc.duration`
- TTS: `TTS_PROVIDER=volc-doubao-tts`
  - 基于火山在线语音合成 HTTP 接口（一次性合成，返回 base64 音频）
  - `TTS_VOLC_RESOURCE_ID` 默认使用 `volc.service_type.10029`
- LLM: 使用 `LLM_PROVIDER=openai-compatible-chat-completions`
  - 将 `LLM_OPENAI_COMPAT_BASE_URL` 指向 Ark（`/api/v3/chat/completions`）并填 `Bearer token`
  - 若目标实现执行严格 OpenAI 参数校验，将 `LLM_OPENAI_COMPAT_TOP_K=0`，避免发送非标准字段

## Current Code Status

- 讯飞 ASR adapter 已实现：
  - WebSocket 鉴权 URL 生成
  - 请求 / 响应 schema
  - base64 结果解码
  - `wpgs` 动态修正文本累积
- bot 启动时根据 `ASR_PROVIDER=xfyun-spark-iat` 选择该 adapter
- bot 已建立 `Opus -> PCM -> 16k mono -> XFYUN WebSocket ASR` 接线骨架
- 流式 transcript 通过 DataChannel `asr.partial / asr.final` 回推客户端
- bot 已建立 `ASR final -> LLM stream -> punctuation_boundary segmenter -> TTS stream` 回复侧骨架
- TTS 已调用真实 provider，并接入 WebRTC 下行音轨
- Opus decoder 已切换至 `github.com/godeps/opus` 的 WASM libopus 后端
- 该后端相比 SILK-only 路线更贴近浏览器真实 WebRTC Opus 兼容面
- 下一步重点是浏览器实流验证、错误分型与 fallback 策略，而非继续更换 decoder

## Required Interfaces

- `ASRAdapter`
- `LLMAdapter`
- `TTSAdapter`

## Rules

- adapter 只关心 provider 协议，不关心 session 状态机
- adapter 输出统一事件或 chunk，不直接改 SessionTask 状态
- provider timeout、cancel、error 必须显式返回
- 优先提供 mock adapter，便于本地开发与协议调试
- 流式 ASR 不得将每个不稳定 partial 直接推进 LLM
- 流式 TTS 必须支持 cancel，并在 interrupt 时优先停止

## Phase Strategy

- Phase 0: interface + mock
- Phase 1: demo provider
- Phase 2: production-grade provider with timeout / retry / observability

## Streaming Adapter Guidance

- Decoder:
  - 位于 WebRTC ingress 与 VAD / ASR 之间
  - 将 codec-aware encoded packet 转换为统一 PCM frame
  - 已切换至 WASM libopus 后端，按 track 独立创建 decoder，避免跨会话共享状态
- ASR:
  - 输入为 decoder 后的持续音频流
  - 输出区分 partial / stable / final
  - 当前目标 provider 为讯飞 `spark_zh_iat`
- LLM:
  - 优先消费 stable / final transcript
  - 输出支持 token stream
  - 通过 `LLM_SEGMENTER_MODE=punctuation_boundary` 在 `。！？；!?;` 等标点处分段
- TTS:
  - 输入支持增量文本或句段
  - 输出为可取消的音频 chunk 流
  - 每个句段单独发起一次 provider synthesis

## Config Notes

- VAD 配置使用 `VAD_*` 前缀
- 讯飞实时听写配置使用 `ASR_XFYUN_*` 前缀
- 火山 ASR 配置使用 `ASR_VOLC_*` 前缀
- OpenAI-compatible LLM 配置使用 `LLM_OPENAI_COMPAT_*` 前缀
- 句段切分配置使用 `LLM_SEGMENTER_*` 前缀
- 讯飞 TTS 配置使用 `TTS_XFYUN_*` 前缀
- 火山 TTS 配置使用 `TTS_VOLC_*` 前缀
- `TTS_XFYUN_PCM_ENDIAN` 用于声明 provider 返回的 `audio/L16` 字节序，默认按 `little` 处理
- `TTS_DEBUG_DUMP_DIR` 打开 TTS 调试导出，按 segment 落 `raw` / `wav` / `txt`
- 调试导出会额外生成 `be_16k / le_16k / be_8k / le_8k` 四个 WAV 变体，便于快速判断真实 sample rate 与 endian
- 真实密钥仅放置于 `.env.local` 或部署环境，不写入 tracked 示例文件
