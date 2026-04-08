# Tests

当前测试策略分为三层：

- 单元测试: 放在 Go 包内，验证协议、状态机、manager
- 协议测试: 验证 signaling / datachannel 消息格式与约束
- smoke / e2e: 未来用于最小 WebRTC 链路验证

Phase 0 先保证状态机和协议结构可测试，再逐步补 WebRTC smoke test。

