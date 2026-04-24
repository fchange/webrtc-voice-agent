# Hotel Booking Demo

## Goal

将当前实时语音底座落成可演示的业务闭环：用户像致电酒店前台一样，与 AI 沟通并完成房间预订。

## Demo Scope

### 1. 房型库存内部服务

提供各房型的实时在线数量，作为唯一业务事实源。

读取接口：

- `GET /internal/room-types`

返回字段：

- `room_type_id`
- `name`
- `description`
- `price_label`
- `capacity`
- `available_count`

### 2. 预订内部服务

提供预订接口，接收必要信息并返回结构化状态。

写入接口：

- `POST /internal/reservations`

请求字段：

- `room_type_id`
- `guest_name`
- `phone_number`

响应字段：

- `reservation_id`
- `status`
- `message`
- `room_type_id`
- `guest_name`
- `phone_number`

状态枚举：

- `confirmed`: 预订成功并扣减库存
- `sold_out`: 提交时已无库存，需引导改选其他房型
- `invalid_input`: 用户信息不完整或格式不合法
- `failed`: 系统异常，需致歉并提示稍后再试

### 3. 运营展示网页

网页至少展示两部分信息：

- 各房型当前余量
- 最新预订记录与状态

该页面作为 demo 控制台与排障观测面。

### 4. LLM Tools / API

对 AI 暴露两类核心能力：

- `list_room_types`: 查询可售房型与实时数量
- `create_reservation`: 以房型、姓名、手机号发起预订

## Business Rules

- AI 在回答房态问询前，必须先调用 `list_room_types`
- AI 不得将 prompt 中的示例文案当作库存事实
- AI 在调用 `create_reservation` 前，必须先确认房型、姓名、手机号
- AI 仅在接口返回 `confirmed` 后，才能告知预订成功
- 若目标房型无库存，AI 必须推荐仍有库存的替代房型，禁止直接结束对话

## Suggested MVP Simplification

首版库存模型简化为单一剩余数量字段，不处理日期库存、支付、退款或 PMS 同步。首版预订定位为即时占房演示，不覆盖完整酒店中台能力。

## Conversation Flow

1. 用户：询问可预订房型
2. AI：调用 `list_room_types`，返回 `available_count > 0` 的房型，同时说明已售罄项
3. 用户：选定房型
4. AI：收集姓名与手机号
5. AI：调用 `create_reservation`
6. 系统：
   - `confirmed` 时返回预订编号
   - `sold_out` 时提示库存已变化
   - `invalid_input` 时指出缺失信息
   - `failed` 时提示稍后再试
7. AI：用自然语言完成收口；`sold_out` 分支继续推荐其他房型

## Data Model Sketch

### Room Type

- `id`
- `name`
- `capacity`
- `price_label`
- `available_count`

### Reservation

- `id`
- `room_type_id`
- `guest_name`
- `phone_number`
- `status`
- `created_at`

## Demo Definition of Done

- 用户能通过语音完成一次真实的库存查询
- 用户能在提供姓名与手机号后发起预订
- 系统返回结构化预订状态，成功时扣减库存
- 网页同步显示库存与预订结果
- AI 能正确处理售罄分支，不会对无房房型继续下单
