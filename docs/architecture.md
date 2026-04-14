# Architecture

## High-level Model

系统采用单机多进程 monorepo，这个边界的目标不是“拆服务而拆服务”，而是把实时语音 Agent 中最容易混在一起的几类职责强制分开：

- `apps/web`: Web 客户端，负责麦克风、PeerConnection、DataChannel 和 UI 状态展示
- `cmd/signal`: Signaling 进程，负责鉴权、创建会话、SDP / ICE 中继、session 与 bot 的路由
- `cmd/bot`: Bot 进程，负责 WebRTC 终端、音频流接入、SessionTask 编排和 adapter 调用
- 规划中的酒店 demo 业务服务: 提供房型库存与预订能力，供 bot tool 层和运营展示页复用

这套拆分使项目可以同时展示：

- WebRTC 实时媒体处理能力
- 自定义 signaling 设计能力
- DataChannel 控制面设计能力
- 流式 AI 编排能力
- tool-driven 业务闭环能力
- provider adapter 插件式架构能力

## Why This Boundary

- WebRTC 音频处理与会话编排属于 bot，不属于 signaling
- signaling 只维护连接关系和协商过程，避免变成业务网关
- SessionTask 作为每个会话的最小调度单元，避免过早引入全局大调度器
- provider 通过 adapter 接口进入，避免 ASR / LLM / TTS 反向污染 session 编排
- 酒店库存与预订属于独立业务事实源，不直接写死在 prompt 或前端 UI

进一步说，这个边界让系统具备几个更适合展示的特性：

- signaling 可以独立演进协议，不影响 bot 会话逻辑
- bot 可以独立演进 provider、状态机和 tool routing
- 业务服务可以独立演进事实模型，而不是绑死在 prompt 中
- 前端可以把 DataChannel 事件直接可视化，作为调试和演示界面

## Process Model

### Signal

- 负责 `create_session`
- 负责客户端鉴权
- 维护会话元数据与 bot 映射
- 转发 `offer / answer / candidate / close`

### Bot

- 为每个 session 维护一个 `SessionTask`
- 接收用户上行音频
- 输出 Bot 下行音频
- 消费和发送 DataChannel 事件
- 执行会话内中断、取消、结束、空闲回收
- 当前 Phase 0 中，bot 已通过 `control` DataChannel 驱动 `session.ready` 与 `turn.interrupt`
- 当前 Phase 0 中，bot 已将上行 WebRTC 音频整理为 codec-aware encoded packet ingress stream，供 endpointing、decoder、VAD 和 ASR 共用
- 在酒店 demo 中，bot 还负责把 LLM 的 tool 调用转成库存查询与预订请求，并把结构化结果转换为对话回复

### Hotel Demo Service

- 提供房型查询接口，返回各房型实时余量
- 提供预订接口，接收房型、姓名、手机号并返回状态
- 作为网页展示页和 bot tool 层共享的业务真相来源
- MVP 可先以内存存储或本地 mock 数据实现，后续再替换为持久化后端

## Data Flow

1. Web 调用 signal 创建 session
2. Web 与 signal 建立 signaling 通道
3. signal 将会话路由给 bot
4. Web 与 bot 完成 WebRTC 协商
5. 用户音频通过 WebRTC track 持续上行至 bot
6. bot 执行权威 VAD / endpointing
7. bot 将控制与状态通过 DataChannel 返回 Web
8. `SessionTask` 编排 VAD / ASR / LLM / TTS
9. 当 LLM 需要业务事实时，通过 tool 层读取库存或发起预订
10. Bot 音频通过 WebRTC track 下行至 Web

这个流程体现的是“媒体流、控制流、业务事实流”三条链路并行但不混杂：

- 媒体流走 WebRTC track
- 控制流走 DataChannel
- 业务事实流走 tool + 内部服务

这也是项目比较适合作为作品集展示的地方，因为它不是单线程线性调用，而是显式建模了多种流。

## VAD Placement

- 服务端 VAD 是权威事实源
- 客户端 VAD 只作为可选 hint 和 UI 辅助
- MVP 阶段不依赖客户端 VAD 做 turn 裁决
- 当前代码已接入服务端 packet-activity endpointing placeholder，用于先跑通服务端裁决链路

这样设计的原因：

- turn 边界、interrupt、cancel、idle timeout 都属于 session 状态机职责
- 这些职责最终必须由 bot 进程统一裁决
- 只靠客户端 VAD 会导致状态漂移、难以复盘、难以做 provider timeout 与异常恢复

## Streaming Strategy

- 上行音频持续传输到 bot，不由客户端提前截流
- bot 先统一落到 encoded audio ingress stream，再进入 endpointing / 解码 / VAD / ASR
- partial ASR 主要服务 UI 与早期反馈
- stable / final ASR 片段再推进 LLM
- LLM 查询房型与预订状态时，必须读取内部服务返回的结构化结果
- LLM 输出尽量流式进入 TTS
- TTS 音频 chunk 流式下行
- interrupt 到来时先 cancel 当前 TTS，再取消本轮未完成下游任务

这条流式链路当前可概括为：

`WebRTC ingress -> endpointing -> decoder -> ASR -> LLM stream -> punctuation segmenter -> TTS -> WebRTC downlink`

它的价值不是“模型接起来了”，而是：

- 明确哪些节点是实时敏感路径
- 明确哪些节点可以替换 provider
- 明确 interrupt 时应该取消哪一段任务
- 明确如何在流式回复里做渐进式披露和 tool 调用

## Progressive Disclosure And Tools

当前酒店 demo 不是让模型“直接回答一个预订结果”，而是通过工具逐步拿事实：

- `list_room_types`: 查询实时房型与库存
- `create_reservation`: 在信息完备时创建预订
- `end_call`: 显式声明“当前回复播完后结束通话”

这样做有几个好处：

- 避免在未查库存前承诺“有房”
- 避免在未写入预订结果前承诺“预订成功”
- 将结束通话从关键词猜测升级为显式控制意图
- 让 LLM 能力按任务需要逐步披露，而不是一次性暴露全部控制权

## Future Evolution

- Signal 可替换为多实例 + Redis session 路由
- Bot 可水平扩容，但每个 session 仍归属单个 bot 实例
- 酒店 demo 服务可从 in-process mock 演进为独立内部 HTTP 服务
- 运营展示页可从简单状态页演进为完整后台
- TURN、指标、追踪、provider 熔断可在后续阶段加入
