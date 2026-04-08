# Hotel Booking Demo

## Goal

把当前实时语音底座落成一个可演示的业务闭环: 用户像打电话给酒店前台一样，与 AI 沟通并完成房间预订。

## Demo Scope

### 1. 房型库存内部服务

提供各房型的实时在线数量，作为唯一业务事实源。

建议最小读取接口:

- `GET /internal/room-types`

建议返回字段:

- `room_type_id`
- `name`
- `description`
- `price_label`
- `capacity`
- `available_count`

### 2. 预订内部服务

提供预订接口，接收最小必要信息并返回结构化状态。

建议最小写入接口:

- `POST /internal/reservations`

建议请求字段:

- `room_type_id`
- `guest_name`
- `phone_number`

建议响应字段:

- `reservation_id`
- `status`
- `message`
- `room_type_id`
- `guest_name`
- `phone_number`

建议状态枚举:

- `confirmed`: 预订成功并扣减库存
- `sold_out`: 提交时已无库存，需引导改选其他房型
- `invalid_input`: 用户信息不完整或格式不合法
- `failed`: 系统异常，需礼貌致歉并建议稍后再试

### 3. 运营展示网页

网页至少展示两块信息:

- 各房型当前余量
- 最新预订记录与状态

这个页面既是 demo 控制台，也是排障和人工观测面。

### 4. LLM Tools / API

给 AI 暴露两类核心能力:

- `list_room_types`: 查询可售房型和实时数量
- `create_reservation`: 以房型、姓名、手机号发起预订

## Business Rules

- AI 在回答“还有没有房”之前，必须先调用 `list_room_types`
- AI 不得把 prompt 里的示例文案当作库存事实
- AI 在调用 `create_reservation` 前，必须先确认房型、姓名、手机号
- AI 只有在接口返回 `confirmed` 后，才能明确告知“预订成功”
- 若目标房型无库存，AI 必须主动推荐仍有库存的替代房型，而不是直接结束对话

## Suggested MVP Simplification

为了先把 demo 跑通，首版库存模型可以先简化为“当前剩余房间数”，不处理复杂日期库存、支付、退款或 PMS 同步。

这意味着首版预订更像“即时占房”演示，而不是完整酒店中台。

## Conversation Flow

1. 用户: 询问有哪些房型还能预订
2. AI: 调用 `list_room_types`，只返回 `available_count > 0` 的房型，必要时也说明已售罄项
3. 用户: 选定房型
4. AI: 收集姓名和手机号
5. AI: 调用 `create_reservation`
6. 系统:
   - 若 `confirmed`，返回预订编号
   - 若 `sold_out`，提示库存已变化
   - 若 `invalid_input`，指出缺失信息
   - 若 `failed`，提示稍后再试
7. AI: 用自然语言完成收口，并在 `sold_out` 时继续推荐其他房型

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
- 用户能提供姓名与手机号后发起预订
- 系统能返回结构化预订状态，并在成功时扣减库存
- 网页能同步显示库存与预订结果
- AI 能正确处理售罄分支，不会继续给已无房型下单
