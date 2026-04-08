# Deployment

## Local Development

本地阶段采用单机多进程：

- `signal` 监听 `:8080`
- `bot` 监听 `:8081`
- `web` 监听 `:5173`

推荐流程：

1. 本地先用公开 STUN
2. 不强依赖 TURN
3. provider 先使用 mock

## Future Production Direction

- signal 多实例无状态化
- bot 多实例，session 定向路由
- TURN 独立部署
- Redis 或服务发现用于 session 路由
- 指标、日志、追踪统一汇聚

## Do Not Do Yet

- Phase 0 不做 k8s 复杂编排
- Phase 0 不做全量 NAT 优化
- Phase 0 不做大规模熔断治理

