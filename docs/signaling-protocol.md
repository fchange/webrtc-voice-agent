# Signaling Protocol

## Transport

- Protocol: WebSocket
- Encoding: JSON
- Version: `v1alpha1`
- WebSocket endpoint: `/ws?session_id=<id>&role=<client|bot>&access_token=<token>`

## Auth

- `POST /v1/sessions` 使用 `Authorization: Bearer <token>`
- `GET /ws` 在 MVP 阶段使用查询参数 `access_token`
- 后续可替换为短期 session token；Phase 0 不引入复杂鉴权系统

## Message Envelope

```json
{
  "version": "v1alpha1",
  "type": "session.offer",
  "session_id": "sess_123",
  "trace_id": "trace_123",
  "payload": {}
}
```

## Required Message Types

- `session.create`
- `session.created`
- `session.attach`
- `session.attached`
- `session.offer`
- `session.answer`
- `session.ice_candidate`
- `session.close`
- `session.error`

## Attach Flow

1. Web 调用 `POST /v1/sessions`
2. signal 为 session 分配 ID，并通知 bot 创建本地 session
3. bot 以 `role=bot` 连接 `/ws`
4. web 以 `role=client` 连接 `/ws`
5. signal 转发 `offer / answer / ice_candidate / close`

## Design Rules

- signal 不解释媒体语义，仅转发协商消息
- signaling 必须携带 `session_id`
- 错误必须落到标准错误码
- 协议新增字段优先保持向后兼容
- signal 可执行 session 到 bot 的映射，但不承担 turn 级业务编排
