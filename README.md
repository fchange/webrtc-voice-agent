# WebRTC Voice Bot

WebRTC 版开源语音对话机器人。

这个仓库当前不是完整产品实现，而是一个适合持续迭代的初始化地基，目标是先把单机 MVP 的工程边界搭对：

- Web 客户端通过 Signaling 与服务端协商 SDP / ICE
- 单用户对单 Bot 的 WebRTC 实时语音链路
- DataChannel 作为控制与状态事件总线
- SessionTask 作为会话内调度核心
- ASR / LLM / TTS 通过 adapter 接口接入
- 中断、取消、结束作为高优先级系统事件处理

## 当前技术选型

- 后端主语言: Go
- WebRTC 服务端方向: Pion
- Web 客户端: TypeScript + React + Vite
- Signaling: WebSocket
- 音频传输: WebRTC audio track
- 控制消息: DataChannel JSON
- 本地开发: 单机多进程

补充判断：

- 长期推荐: `Go + Pion`
- 最快验证推荐: `Python + aiortc`
- 当前仓库初始化选择: 固定为 `Go + Pion` 路线，不在 Phase 0 同时维护两套后端

原因很直接：这个项目的核心复杂度在 WebRTC、会话编排和中断模型，不在 provider SDK 调用。初始化阶段优先把实时链路和会话边界稳定下来，比短期追求 Python 原型速度更重要。

## 仓库结构

```text
.
├── apps/
│   └── web/                  # Web 客户端
├── cmd/
│   ├── bot/                  # Bot 进程入口
│   └── signal/               # Signaling 进程入口
├── docs/                     # 产品、架构、协议、任务、AI 规范
├── examples/
│   └── env/                  # 本地示例配置
├── internal/
│   ├── adapters/             # ASR / LLM / TTS adapter 接口与 mock
│   ├── app/                  # signal/bot 进程装配
│   ├── config/               # 配置加载
│   ├── logging/              # 结构化日志
│   ├── observability/        # 指标预留
│   ├── protocol/             # signaling / datachannel / error code
│   └── session/              # SessionManager / SessionTask
├── pkg/
│   └── events/               # 统一事件抽象
├── scripts/                  # 本地开发与检查脚本
└── tests/                    # 跨模块测试说明与未来 e2e 入口
```

## 快速开始

### 1. 后端

```bash
make test-go
make run-signal
make run-bot
```

或者直接一键起三端：

```bash
./scripts/dev-all.sh
```

脚本行为：

- 默认优先加载仓库根目录 `.env.local`
- 同时启动 `signal`、`bot`、`web`
- 前台输出三端日志，并带 `[signal]` / `[bot]` / `[web]` 前缀
- `Ctrl+C` 会一起停止全部子进程

默认端口：

- Signal: `:8080`
- Bot: `:8081`

### 2. Web 客户端

```bash
pnpm install
make run-web
```

默认地址：

- Web: `http://localhost:5173`

## 当前可运行范围

当前仓库已经打通的能力：

- 真实 WebSocket signaling
- bot 侧最小 Pion `PeerConnection`
- Web 端真实 `offer / answer / ice` 协商
- `control` DataChannel 双向事件流
- 服务端权威 endpointing placeholder
- `Opus -> PCM -> 16k mono -> XFYUN ASR` 接线骨架
- 浏览器实测可收到 `session.ready / turn.started / vad.started / asr.final`
- SessionManager / SessionTask 最小状态机
- 文档、脚本、测试和 smoke 联调工具

当前仍然是 Phase 0/1 边界，尚未完成的关键项：

- 真实 PCM VAD 替换 packet-activity endpointing
- 有效 transcript 质量验证与 ASR 调试日志
- 多轮对话上下文与 prompt 管理
- 更稳的 TTS 播放缓冲与异常恢复
- 更完整的会话关闭、异常恢复和可观测性

这意味着项目已经不是空骨架，而是一个“控制面、协商面、媒体上行都已落地”的可继续演进起点。

## 文档入口

- [产品定义](./docs/product.md)
- [系统架构](./docs/architecture.md)
- [路线图](./docs/roadmap.md)
- [当前任务](./docs/tasks.md)
- [关键决策](./docs/decisions.md)
- [验收标准](./docs/acceptance.md)
- [会话状态机](./docs/session-state-machine.md)
- [Signaling 协议](./docs/signaling-protocol.md)
- [DataChannel 协议](./docs/datachannel-protocol.md)
- [AI 开发规范](./docs/ai-dev-spec.md)
- [部署说明](./docs/deployment.md)
- [Provider Adapter 规范](./docs/provider-adapters.md)
- [VAD 与 Endpointing](./docs/vad-endpointing.md)

## 初始化阶段的第一优先级

1. 用真实 WebSocket 打通 `create_session -> offer -> answer -> ice` 最小链路
2. 在 bot 进程引入最小 Pion PeerConnection
3. 建立 DataChannel 控制面
4. 建立服务端权威 VAD 与 endpointing
5. 将 `interrupt` 实现为高优先级系统事件，而不是前端按钮行为
