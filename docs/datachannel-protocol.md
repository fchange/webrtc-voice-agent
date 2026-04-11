# DataChannel Protocol

## Transport

- Channel: `control`
- Encoding: JSON
- Version: `v1alpha1`

## Message Classes

- session events: `session.ready`, `session.ending`
- turn events: `turn.started`, `turn.interrupt_hint`, `turn.interrupt`, `turn.cancelled`, `turn.end_of_utterance`, `turn.completed`
- bot events: `bot.speaking.started`, `bot.speaking.stopped`
- status events: `vad.started`, `vad.stopped`
- transcript events: `asr.partial`, `asr.final`
- llm events: `llm.partial`, `llm.final`
- tts events: `tts.segment.started`, `tts.segment.completed`
- error events: `error`

## Envelope

```json
{
  "version": "v1alpha1",
  "type": "turn.interrupt",
  "session_id": "sess_123",
  "turn_id": 3,
  "request_id": "req_123",
  "payload": {
    "reason": "user_barge_in"
  }
}
```

## Rules

- `turn.interrupt_hint` 可以由客户端发送，但不是最终裁决
- `turn.interrupt` 是高优先级消息，必须由服务端确认后生效
- `turn.end_of_utterance` 只能由服务端发送
- `session_id` 必填
- `turn_id` 对 turn 级事件必填
- bot 发出的状态事件必须尽量幂等
- `asr.partial` 和 `asr.final` 由服务端产生，客户端不得伪造
- `llm.partial` 表示 LLM token/chunk 级流式输出，`llm.final` 表示本轮聚合后的最终文本
- `tts.segment.started` / `tts.segment.completed` 以句段为单位，便于调试 punctuation-boundary 分段行为

## Current Phase 0 Behavior

- bot 在 `control` DataChannel 打开后会发送 `session.ready`
- 若下行音频与 response runtime 已就绪，bot 可在 `session.ready` 后主动发起 opening turn，用于接通后的自我介绍
- 客户端发送 `turn.interrupt_hint` 后，bot 会在当前 placeholder turn 上提升为 `turn.interrupt`
- bot 当前会在真实回复链开始/结束时发送 `bot.speaking.started` 和 `bot.speaking.stopped`
- bot 当前会基于上行 RTP 活动与静默超时发送 `vad.started / vad.stopped / turn.end_of_utterance`
- bot 当前会在 Opus 解码和 PCM 归一化成功时，把流式 ASR 结果通过 `asr.partial / asr.final` 回推到 DataChannel
- bot 当前会在 `ASR final -> LLM stream -> punctuation_boundary -> TTS` 路径上发送 `llm.*` 与 `tts.segment.*`
- 当前 TTS 已调用真实 provider，但还没有把合成音频回推到 WebRTC 下行音轨
