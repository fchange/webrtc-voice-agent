# Product

## Goal

`WebRTC Voice Bot` 是一个面向网页、桌面端、移动端和未来硬件终端的开源实时语音对话系统。首要目标不是聊天 UI，而是稳定的低延迟语音双工链路和清晰的会话编排模型。

## MVP Scope

- Web 客户端优先
- 单用户对单 Bot
- WebRTC 音频上行与下行
- WebSocket signaling
- DataChannel 控制与状态事件
- 基础鉴权
- 中断、取消、结束
- Session 生命周期管理
- mock provider 与真实 provider adapter 扩展点

## Target Users

- 最终用户: 与 Bot 进行实时语音对话
- 部署者: 配置 signaling、bot、provider 和基础网络
- 开发者: 扩展协议、provider、状态机和客户端
- 维护者: 维护模块边界、文档、脚本和测试

## Non-goals For Phase 0

- 多人房间
- SFU / MCU
- 多租户后台
- 长期记忆系统
- 大规模分布式调度
- 企业级权限模型

## Product Constraints

- 中断是一等公民
- Signaling 保持薄层
- Session 内调度优先于全局调度
- 统一事件流优先于 provider 特定逻辑
- MVP 阶段优先稳定链路，不提前做复杂平台能力

