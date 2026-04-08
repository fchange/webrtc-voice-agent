# Roadmap

## Phase 0: Groundwork

- 建立 monorepo 结构
- 固化核心边界: signal / bot / session / protocol / adapters
- 写清文档、任务、决策和 AI 开发规范
- 提供可运行 stub、测试骨架和本地脚本

## Phase 1: MVP Link

- WebSocket signaling 最小实现
- Pion PeerConnection 最小实现
- DataChannel 消息闭环
- 服务端权威 VAD 与 endpointing
- 用户音频上行和 Bot 静音 / mock 音频下行
- interrupt -> cancel current turn -> start next turn

## Phase 2: Real-time Voice Loop

- 接入 VAD
- 接入 mock / demo ASR
- 接入 LLM adapter
- 接入 TTS adapter
- 首轮完整实时语音对话闭环

## Phase 3: Hotel Booking Demo

- 定义房型与预订领域模型
- 提供房型实时库存内部服务
- 提供带姓名和手机号的预订接口
- Web 端增加库存与预订展示页
- 给 LLM 暴露库存查询与预订 tools
- 实现“无房自动排除并推荐替代房型”的对话闭环
- 完成一次可演示的“电话问房并预订”端到端流程

## Phase 4: Production Hardening

- TURN 支持
- 多 bot 实例路由
- Redis / service registry
- 指标、追踪、告警
- provider timeout / retry / circuit breaker
