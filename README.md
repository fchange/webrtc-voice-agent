# WebRTC AI Voice Agent

## 1. 项目概述

`WebRTC AI Voice Agent` 是一个面向实时语音交互场景的工程化示例项目，重点覆盖 WebRTC 媒体链路、会话编排、流式 AI 调用、工具调用以及业务闭环等关键能力。

项目当前以“电话预订酒店房间”作为首个垂直演示场景，用于验证实时语音 Agent 在以下方面的工程边界：

- 实时音频链路建立与控制
- 自定义信令协议设计
- DataChannel 控制面建模
- 流式 ASR / LLM / TTS 编排
- Tool Calling 驱动的业务事实获取
- Provider Adapter 插件式扩展
- 会话状态机、中断与结束控制

该项目既可作为实时语音 Agent 的技术样例，也可作为后续扩展至客服、前台、语音表单、硬件终端等场景的系统基础。

## 2. 核心能力

### 2.1 WebRTC 实时音频链路

- 支持浏览器端麦克风音频采集
- 支持 Web 端 `offer / answer / ice_candidate` 协商流程
- 支持 Bot 侧 `PeerConnection` 建立与维护
- 支持音频媒体流与控制流分离

### 2.2 自定义 Signaling 协议

- 采用 WebSocket 作为信令传输通道
- 定义独立的会话创建与协商消息格式
- 明确 `client / bot / signal` 三方职责边界
- 支持协议版本化与向后兼容扩展

### 2.3 DataChannel 控制面

- 使用 `control` DataChannel 承载控制与状态事件
- 统一表达 `session.* / turn.* / asr.* / llm.* / tts.*` 事件
- 支持联调、可视化和调试过程中的状态观测

### 2.4 流式 AI 编排

- 支持 `ASR final -> LLM stream -> punctuation boundary -> TTS` 的流式处理链路
- 支持 token/chunk 级别输出
- 支持句段级 TTS 合成与播放
- 支持回复链路的取消、中断和结束控制

### 2.5 Provider Adapter 插件式扩展

- 定义统一的 `ASRAdapter / LLMAdapter / TTSAdapter` 接口
- 支持 mock provider 与真实 provider 的替换
- 支持 OpenAI-compatible、讯飞、火山等 provider 接入
- 避免核心会话编排与单一供应商 SDK 耦合

### 2.6 Tool Calling 与渐进式能力披露

- 通过 `list_room_types` 查询结构化库存信息
- 通过 `create_reservation` 执行预订写入
- 通过 `end_call` 控制本轮回复完成后的会话结束
- 将业务事实读取与模型文本生成分离，降低幻觉风险

### 2.7 会话状态机与中断控制

- 定义 `created / negotiating / active / processing / responding / closing / closed` 等状态
- 将 `interrupt` 建模为高优先级系统事件
- 支持 bot speaking、用户插话、turn cancel、会话结束等关键流程
- 支持“本轮播报完成后结束通话”的延迟结束策略

## 3. 系统特性

- 面向实时双工语音交互场景设计
- 采用服务端权威 turn 裁决模型
- 将媒体流、控制流和业务事实流分离建模
- 采用插件式 provider 扩展机制
- 支持垂直业务通过 tool 层接入
- 采用单机多进程边界划分，便于联调与后续扩展

## 4. 系统架构

系统采用单机多进程 monorepo 架构，主要模块如下：

- `apps/web`
  Web 客户端，负责麦克风采集、PeerConnection、DataChannel 与界面状态展示。
- `cmd/signal`
  Signaling 进程，负责鉴权、创建会话、SDP / ICE 中继以及 session 与 bot 路由。
- `cmd/bot`
  Bot 进程，负责 WebRTC 终端、音频接入、SessionTask 编排、工具调用与 provider 调度。
- `internal/session`
  会话生命周期管理与 turn 编排核心。
- `internal/adapters`
  ASR / LLM / TTS provider 的统一接入层。
- `internal/hotel`
  酒店库存与预订领域服务，作为演示场景中的业务事实源。

相关说明见 [docs/architecture.md](./docs/architecture.md)。

## 5. 端到端流程

1. Web 客户端调用 signal 创建 session。
2. Web 与 bot 通过 signal 完成 `offer / answer / ice_candidate` 协商。
3. 浏览器麦克风音频通过 WebRTC 持续上行。
4. Bot 接收音频并执行 endpointing、解码和 ASR。
5. `ASR final` 结果触发 LLM 流式回复。
6. LLM 按需调用库存查询、预订和结束通话相关 tool。
7. 回复文本按标点分段进入 TTS。
8. TTS 音频通过 WebRTC 下行返回客户端。
9. DataChannel 同步会话状态、turn 事件与调试信息。
10. 服务端在用户插话或结束条件满足时执行统一状态裁决。

## 6. 技术设计要点

- Signaling 进程保持薄层路由职责，不承担会话级业务编排。
- Bot 进程负责实时音频处理、状态机执行与下游任务协调。
- DataChannel 作为控制面总线，不与媒体通道混用。
- Tool 层负责获取结构化业务事实，避免依赖 prompt 假设。
- Adapter 层负责隔离 provider 差异，降低核心模块耦合。
- 服务端负责最终的 interrupt、cancel、end 等状态裁决。

## 7. 当前版本范围

当前版本可视为项目 `1.0`，已具备以下能力：

- WebSocket signaling 与会话创建链路
- WebRTC 协商闭环
- `control` DataChannel 双向事件流
- 服务端 endpointing placeholder
- `Opus -> PCM -> 16k mono -> ASR` 接线骨架
- `ASR -> LLM -> TTS` 流式回复运行时
- Provider Adapter 机制与真实 provider 接入
- 酒店库存查询、预订、结束通话 tool
- SessionManager / SessionTask / response runtime 核心骨架

当前版本的重点在于建立清晰、可扩展、可验证的实时语音 Agent 工程边界。后续演进方向见 [docs/roadmap.md](./docs/roadmap.md)。

## 8. 技术栈

- 后端：Go
- WebRTC：Pion
- 前端：TypeScript + React + Vite
- Signaling：WebSocket
- 控制面：DataChannel JSON
- Provider 接入：Adapter Pattern
- 本地开发方式：单机多进程

## 9. 仓库结构

```text
.
├── apps/
│   └── web/                  # Web 客户端
├── cmd/
│   ├── bot/                  # Bot 进程入口
│   └── signal/               # Signaling 进程入口
├── docs/                     # 架构、协议、路线图、状态机等文档
├── internal/
│   ├── adapters/             # ASR / LLM / TTS provider adapters
│   ├── app/                  # signal / bot 装配与 runtime
│   ├── audio/                # 音频解码与 PCM 流处理
│   ├── hotel/                # 酒店库存与预订领域服务
│   ├── protocol/             # signaling / datachannel 协议
│   └── session/              # SessionManager / SessionTask
├── scripts/                  # 本地开发与检查脚本
└── tests/                    # 测试与后续 e2e 扩展入口
```

## 10. 快速开始

### 10.1 启动后端

```bash
make test-go
make run-signal
make run-bot
```

或使用一键脚本启动多端：

```bash
./scripts/dev-all.sh
```

默认端口：

- Signal: `:8080`
- Bot: `:8081`

### 10.2 启动 Web 客户端

```bash
pnpm install
make run-web
```

默认地址：

- Web: `http://localhost:5173`

## 11. 文档索引

- [系统架构](./docs/architecture.md)
- [会话状态机](./docs/session-state-machine.md)
- [Signaling 协议](./docs/signaling-protocol.md)
- [DataChannel 协议](./docs/datachannel-protocol.md)
- [Provider Adapter 规范](./docs/provider-adapters.md)
- [路线图](./docs/roadmap.md)

## 12. Roadmap 摘要

- `1.0`
  建立实时语音 Agent 的基础工程边界与核心链路。
- `1.1`
  强化语音质量、VAD、异常恢复与运行稳定性。
- `1.2`
  完善酒店演示场景与运营展示能力。
- `1.3`
  增强可观测性、调试能力与测试覆盖。
- `2.0`
  面向生产运行时补齐 TURN、多实例路由、追踪与容错能力。

详细规划见 [docs/roadmap.md](./docs/roadmap.md)。

## 13. 项目定位

本项目的定位不是通用聊天演示页面，而是一个围绕实时语音交互构建的工程化 Agent 样例。其重点在于协议边界、状态机设计、流式链路编排、工具调用约束以及面向替换的系统扩展能力。
