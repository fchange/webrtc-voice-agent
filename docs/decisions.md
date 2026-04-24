# Decisions

## ADR-0001: Backend Language

- Decision: 初始化阶段固定后端主语言为 Go
- Reason:
  - 项目长期目标为开源、稳定、可演进的实时语音系统
  - WebRTC 是核心路径，Go + Pion 接近长期形态
  - 并发模型与资源占用适配后续 production hardening
- Rejected:
  - 同时维护 Go 与 Python 两套后端
  - Phase 0 先写 Python，后续整体迁移至 Go

## ADR-0002: Monorepo

- Decision: 使用 monorepo 管理 web、backend、docs、examples、scripts
- Reason:
  - 协议、文档、脚本与任务状态需要单仓协同演进
  - 适配开源维护与 AI 持续开发
  - 单用户单 bot MVP 无需拆分多仓库

## ADR-0003: SessionTask First

- Decision: 不引入全局调度器，以每 session 一个 `SessionTask` 为核心
- Reason:
  - 中断、取消、空闲回收属于会话内问题
  - 优先保证局部正确性，全局调度延后

## ADR-0004: Thin Signaling

- Decision: signaling 仅负责鉴权、会话创建、协商与路由
- Explicitly Not In Signal:
  - 音频处理
  - provider 编排
  - turn 级业务逻辑

## ADR-0005: Server-side Authoritative VAD

- Decision: VAD 与 endpointing 由 bot 进程权威裁决，客户端 VAD 仅作辅助 hint
- Reason:
  - turn 边界与 interrupt/cancel 同属 session 状态机
  - 服务端需统一控制流式 ASR、流式 TTS 与异常恢复
  - 便于日志、回放、指标与问题定位
- Client-side VAD Role:
  - speaking UI
  - 本地音量电平
  - 后续的快速 barge-in hint

## ADR-0006: First Vertical Demo Is Hotel Phone Booking

- Decision: 当前仓库的首个业务化演示场景固定为「电话预订酒店房间」
- Reason:
  - 相比通用聊天 bot，更易定义清晰的成功标准
  - 需要真实 tool 调用，可验证语音 agent 的业务闭环能力
  - 房型库存与预订结果适配结构化网页展示
- Implications:
  - 房型库存与预订状态必须来自内部服务
  - bot 需提供面向 LLM 的库存查询与预订 tools
  - 无房分支作为一等对话路径处理，避免失败后再补救
