# Session State Machine

## Core Principle

中断不是 UI 辅助动作，而是高优先级系统事件。它必须能够打断当前 bot 输出、取消当前 turn 的未完成任务，并切换到新 turn。

## Session States

- `created`: session 已分配，尚未完成协商
- `negotiating`: 正在交换 SDP / ICE
- `active`: 音频链路可用，等待用户输入
- `processing`: 正在处理当前用户 turn
- `responding`: bot 正在输出语音或事件
- `closing`: 正在结束会话
- `closed`: 已结束

## System Events

- `session.start`
- `session.ready`
- `vad.started`
- `vad.stopped`
- `turn.begin`
- `turn.interrupt_hint`
- `turn.interrupt`
- `turn.cancel`
- `turn.end_of_utterance`
- `turn.complete`
- `session.timeout`
- `session.end`

## Interrupt Rule

收到 `turn.interrupt` 时：

1. 停止当前 TTS 输出
2. 取消当前 turn 下游任务
3. 标记当前 turn 为 interrupted
4. 切换到下一 turn
5. session 返回 `active`，准备接收新的用户输入

## Interrupt Hint Rule

`turn.interrupt_hint` 可以由客户端发送，但它不是最终裁决。

bot 收到 hint 后：

1. 立即提升本会话对用户上行音频的关注级别
2. 结合服务端 VAD 和当前 bot speaking 状态做最终判断
3. 只有判定用户真实 barge-in 时，才升级为 `turn.interrupt`

## End Of Utterance Rule

`turn.end_of_utterance` 只能由服务端产生。它表示：

- 当前用户语音段已经结束
- 可以将稳定音频片段推进 ASR finalization 或下游 LLM
- 它不是前端按钮事件

## State Constraints

- `closed` 后不可再进入其他状态
- `closing` 只允许进入 `closed`
- `responding` 可被 `turn.interrupt` 抢占并回到 `active`
