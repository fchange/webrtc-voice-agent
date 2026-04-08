# Tasks

## Active

| ID | Priority | Status | Task |
| --- | --- | --- | --- |
| P0-001 | P0 | done | 初始化 monorepo、文档骨架、脚本、Go 服务 stub |
| P0-002 | P0 | done | 实现 WebSocket signaling 最小协议闭环 |
| P0-003 | P0 | doing | 在 bot 中接入最小 Pion PeerConnection |
| P0-004 | P0 | doing | 建立 DataChannel `session.ready / turn.interrupt / session.end` |
| P0-005 | P0 | doing | 建立服务端权威 VAD / endpointing 骨架 |
| P0-006 | P0 | doing | 整理 bot 上行音频为 codec-aware ingress stream 与 decoder 边界，供 endpointing / VAD / ASR 复用 |
| P0-007 | P0 | todo | 将 SessionTask 接入真实会话生命周期 |
| P0-008 | P0 | doing | 接入讯飞 WebSocket ASR adapter 骨架与鉴权配置 |
| P0-009 | P0 | doing | 建立 Opus -> PCM -> 16k mono 音频桥，并把流式 transcript 接到 DataChannel |
| P0-010 | P0 | doing | 将 Opus decoder 升级为浏览器更兼容的后端，并补充按 track 独立 decoder 生命周期 |
| P0-011 | P0 | doing | 接入 OpenAI-compatible 流式 LLM、punctuation-boundary 句段切分与讯飞 TTS 回复链 |

## Next

| ID | Priority | Status | Task |
| --- | --- | --- | --- |
| P1-001 | P1 | todo | 将 TTS 合成音频真正回推到 WebRTC 下行音轨 |
| P1-002 | P1 | todo | Web 端接入真实麦克风和远端音频播放 |
| P1-003 | P1 | todo | 增加空闲超时与 graceful end |
| P1-004 | P1 | todo | 增加结构化错误码与前后端映射 |
| P1-005 | P1 | todo | 客户端加入本地 VAD hint 与 speaking meter |

## Rules

- `P0` 任务优先保证链路闭环，不扩 scope
- 协议变更必须同步文档与测试
- 中断相关任务优先于“更聪明的 Bot”能力
- VAD 以服务端判定为准，客户端 hint 不得直接决定 turn 完成
