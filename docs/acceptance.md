# Acceptance

## Phase 0 Acceptance

- 仓库结构清晰，目录职责明确
- Signal / Bot 可本地启动
- SessionTask 具备基础状态迁移与 interrupt 语义
- 协议文档与代码结构一致
- README、docs、examples、scripts 完整可读

## Phase 1 Acceptance

- Web 可建立 session 并完成最小 signaling
- Bot 可建立最小 WebRTC PeerConnection
- DataChannel 可双向发送控制事件
- 服务端可产生 `vad.started / vad.stopped / turn.end_of_utterance`
- 用户可触发 interrupt，系统能取消当前 turn 并进入下一轮

## Phase 2 Acceptance

- 完成用户语音上行 -> 识别 -> LLM -> TTS -> Bot 语音下行闭环
- provider 超时、取消、空闲结束具备可观测性
- 关键链路具备协议测试、状态机测试和 smoke test

## Phase 3 Acceptance

- 系统可返回各房型实时在线数量
- 系统可接收房型、姓名、手机号并返回结构化预订状态
- 成功预订后，网页和接口可观察到库存同步扣减
- 网页可展示当前房型余量和最新预订记录
- AI 在房型售罄时不会继续下单，并会推荐仍可预订的其他房型
