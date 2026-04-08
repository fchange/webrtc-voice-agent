# Product

## Goal

`WebRTC Voice Bot` 是一个面向网页、桌面端、移动端和未来硬件终端的开源实时语音对话系统。首要目标不是聊天 UI，而是稳定的低延迟语音双工链路和清晰的会话编排模型。

这个仓库的首个垂直 demo 场景明确为“电话预订酒店房间”:

- 用户通过语音询问房型和余量
- AI 前台通过内部服务查询实时库存
- AI 与用户沟通，收集姓名和手机号
- AI 调用预订接口并返回预订状态
- 运营人员可在网页里看到库存与预订结果

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
- 酒店房型实时库存查询
- 带姓名和手机号的房间预订接口
- 运营展示网页
- 给 LLM 使用的库存查询与预订 tools
- AI 主动排除无房源并推荐可选房型

## Target Users

- 最终用户: 通过电话与 AI 前台沟通并预订酒店房间
- 酒店运营: 在网页中查看房型余量和预订结果
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
- 完整酒店 PMS / CRS 对接
- 支付、退款、订单履约
- 复杂日期库存、房价日历和超售策略

## Product Constraints

- 中断是一等公民
- Signaling 保持薄层
- Session 内调度优先于全局调度
- 统一事件流优先于 provider 特定逻辑
- MVP 阶段优先稳定链路，不提前做复杂平台能力
- 房型余量和预订状态必须来自内部服务，而不是 prompt 假设
- AI 不能在未查库存前承诺“有房”
- AI 不能在未收到预订接口成功响应前承诺“订房成功”
