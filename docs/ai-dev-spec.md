# AI Dev Spec

## Primary Goal

后续 AI 代理的首要任务是持续保持边界清晰、协议一致、状态机稳定，而非增加代码产出量。

## Non-negotiable Rules

- 优先维护 `signal / bot / session / adapters / protocol` 的边界
- 不要把 provider 逻辑写进 signaling
- 不要把中断逻辑只做在前端
- 协议改动必须同步更新文档与测试
- 新增目录或文件必须有明确职责
- 不提前引入分布式大架构
- 酒店库存与预订状态必须来自内部服务或 tool 调用，不得写死在 prompt
- AI 不得在未读到结构化库存结果前承诺“有房”
- AI 不得在预订接口未返回成功前承诺“订房成功”

## Preferred Workflow

1. 先读 `docs/tasks.md`
2. 再读 `docs/hotel-booking-demo.md`
3. 再读协议文档与状态机文档
4. 修改前确认是否触及模块边界
5. 先写最小闭环，再补测试和文档

## Coding Guidance

- 新增 provider 必须走 adapter 接口
- 新增会话事件必须归类到统一 event/frame 模型
- 需要跨语言同步的协议变更，先以 docs 为准再改代码
- WebRTC 相关改动优先补 smoke test 或最小可验证脚本
