# Roadmap

## Version 1.0

- WebSocket signaling 与会话创建链路
- WebRTC 协商与媒体链路建立
- `control` DataChannel 控制与状态事件总线
- Session 状态机与 turn 编排机制
- `ASR -> LLM -> TTS` 流式处理链路
- Provider Adapter 扩展机制
- 酒店库存查询、预订与结束通话工具能力
- 酒店库存与预订展示页面
- 预订记录与基础运营视图

## Version 1.1

- 真实 VAD 与 endpointing 优化
- ASR 准确率与端到端语音质量验证
- TTS 播放缓冲、取消与恢复机制优化
- 错误处理、日志与可观测性增强

## Version 1.2

- 房型推荐与无房替代流程优化
- 端到端演示路径稳定性增强

## Version 2.0

后续版本将面向生产运行环境补充以下能力：

- TURN 支持
- 多 bot 实例路由
- 服务发现或集中式会话路由
- 指标、追踪与告警体系
- Provider 超时、重试与熔断治理
