# token-router

基于 Token 预算的路由服务。集群有 N 个节点、每节点预算 M tokens，对外暴露 `/alloc` 与 `/free` 两个 HTTP 接口，保证任意并发下不超卖、不错误释放，无资源时返回 429。

技术栈：Go 1.22 + Gin。无外部存储依赖，状态全部在进程内。

## 快速开始

```bash
# 启动（默认 N=2, M=300, listen :8080）
go run . -n=2 -m=300 -addr=:8080

# 单元 + 集成测试
go test ./...

# 并发安全测试（必须通过）
go test -race ./...
```

## HTTP 接口

响应体严格遵循题面契约：成功返回业务字段，失败返回 `{"error": "<reason>"}`。

### POST /alloc

```bash
curl -X POST http://localhost:8080/alloc \
  -H 'Content-Type: application/json' \
  -d '{"request_id":"req-1","token_count":80}'

# 200 OK
{"node_id":0,"remaining_quota":220}

# 429 Too Many Requests （所有节点都放不下）
{"error":"overloaded"}

# 400 Bad Request （token_count <= 0 或 > M，或缺字段）
{"error":"invalid_request"}
```

幂等性：同一 `request_id` 重复 `/alloc` 会返回原分配（不会重复扣减），并在响应头加 `X-Allocation-Duplicate: true` 作为旁路标识，body 不污染题面契约。

### POST /free

```bash
curl -X POST http://localhost:8080/free \
  -H 'Content-Type: application/json' \
  -d '{"request_id":"req-1"}'

# 200 OK
{"node_id":0}

# 404 Not Found
{"error":"not_found"}
```

### GET /debug/status（辅助）

```bash
curl http://localhost:8080/debug/status
# {"nodes":[{"id":0,"remaining":300},{"id":1,"remaining":300}],"in_flight":0,"budget":300}
```

## 项目结构

按"协议层 → 业务层 → 数据层"三层组织：

```
.
├── main.go                            程序入口
├── conf/                              配置
├── server/                            协议层
│   ├── server.go                        http.Server + 优雅停机
│   ├── router.go                        路由清单
│   └── middleware/recovery.go           panic 恢复
├── app/
│   ├── controller/token/              协议层 handler + 响应封装
│   ├── logic/allocator/               业务层：分配策略
│   └── models/token/                  DTO
├── global/
│   ├── berror/                          统一错误类型 + HTTP 状态映射
│   └── consts/                          常量
└── repository/store/                  数据层
    ├── store.go                         Store interface（便于换 Redis/DB）
    └── memory.go                        进程内实现
```

依赖方向严格自上而下：controller 只依赖 logic，logic 只依赖 store interface，store 实现细节不外泄。

## 设计要点

### 1. 并发安全：CAS + sync.Map，无全局锁

每节点的 remaining quota 用 `atomic.Int64` 持有；`request_id → allocation` 账本用 `sync.Map`。

`/alloc` 路径：
1. **幂等检查**：`Load(request_id)` 命中直接返回原分配 + `Duplicate=true`
2. **快照 + best-fit 排序**：扫一次所有节点的 remaining，过滤可承载本次请求的候选，按剩余升序排列
3. **CAS 扣减**：依次对候选节点 `CompareAndSwap(old, old-tokens)`；首个成功者通过 `LoadOrStore` 写入账本
4. **同 ID 竞态防御**：若 `LoadOrStore` 返回 loaded=true，说明同一 request_id 并发抢入，把刚扣减的 token 加回节点，返回另一方的分配作 duplicate
5. **CAS 全败时重试**：每次失败意味着别人推进了状态，最多 32 次快照重试，每次失败间 `runtime.Gosched()` 让出 P
6. **真无候选**：返回 `ErrOverloaded` → HTTP 429

`/free` 路径：`LoadAndDelete(request_id)` 单步原子取出并删除，命中才把 token 加回节点。这一步同时关掉了 double-free 漏洞：第二次 free 必然 miss → 404。

### 2. 资源利用率：Best-Fit

候选排序时按 remaining 升序，优先用"刚好够"的节点，把"刚开局的大节点"留给后续大请求。在异构请求规模下经验上优于 round-robin / first-fit / worst-fit。

举个例：`N=2, M=300`，先后到达 80、120、200、300、250 的请求 + 中间穿插 free —— 用 best-fit 能完整放下 300 这个大请求；用 round-robin 则极易把两节点都打散到 < 300 的碎片，被迫返回 429。

### 3. CAS 重试策略：用更宽的耐心换更少的伪 429

每次 CAS 失败都意味着另一个 goroutine 完成了一次 reservation，状态已变化，retry 几乎总是能在新快照下找到机会。重试上限 32 次主要是防御真正的 livelock，正常并发下远远跑不到。两次重试之间 `runtime.Gosched()` 让出执行权降低 cache-line 抖动。

不接受"改回全局锁"的取舍：那会丢掉跨节点的并行扣减能力，对资源利用率有反效果。

### 4. 幂等 alloc 的契约设计

业务上需要支持客户端重试，因此同一 `request_id` 二次 `/alloc` 返回原分配（无新扣减），但题面响应字段是固定的两个，所以 duplicate 信号通过 HTTP header `X-Allocation-Duplicate: true` 旁路传递 —— **body 严格符合题面契约，duplicate 在 header 上对监控/调用方可见。**

### 5. 边界与错误码一览

| 场景 | HTTP | body |
|---|---|---|
| 正常分配 | 200 | `{"node_id":0,"remaining_quota":220}` |
| 同 ID 幂等命中 | 200 | 同上 + header `X-Allocation-Duplicate: true` |
| 资源不足 | 429 | `{"error":"overloaded"}` |
| 参数无效（缺字段、≤0、> M） | 400 | `{"error":"invalid_request"}` |
| 正常释放 | 200 | `{"node_id":0}` |
| 释放未知 ID / 二次 free | 404 | `{"error":"not_found"}` |
| 服务 panic | 500 | `{"error":"internal_error"}` |

## 测试覆盖

`go test -race ./...` 必须通过。关键用例：

| 用例 | 验证点 |
|---|---|
| `TestAlloc_BasicAndOverload` | 单节点跑满后再 alloc 返回 overloaded；free 后 remaining 还原 |
| `TestAlloc_BestFit` | N=3,M=100，alloc 30 落在节点 X 后，alloc 70 也落在 X（best-fit），其他节点保留完整容量 |
| `TestAlloc_NoOversellConcurrent` | 200 goroutine 同时 alloc 1 token，节点容量 100，成功数恰好 100，remaining 归 0 |
| `TestAlloc_SameIDConcurrent` | 200 goroutine 用同一 request_id alloc 40，恰好 1 次 fresh + 199 次 duplicate，所有结果 node_id 一致，扣减恰好一次 |
| `TestAlloc_MixedConcurrent` | 50 goroutine × 200 op 随机 alloc/free，结束后所有节点 remaining 回到 M，in_flight 归零 |
| `TestFree_Double` | 重复 free 不会把 quota 加超过初始值 |
| `TestHTTP_ExampleScript` | 题面 10 步示例完整跑通，并显式断言响应中**不出现** `errno/errmsg/data/duplicate` 等非题面字段 |
| `TestHTTP_DuplicateViaHeader` | 二次 alloc 返回 200，body 不变，仅 `X-Allocation-Duplicate: true` 头部出现 |

## 设计决策记录（FAQ）

**为什么不用全局 Mutex？** 跨节点扣减天然没有共享冲突，全局锁会把这条好的并行路径串行化，损害利用率。CAS + sync.Map 既避免锁，又把并发竞争集中在单节点单点（CAS）和单 key 单点（LoadOrStore），可证明无超卖且高吞吐。

**为什么 best-fit 而不是 round-robin？** 题目把"资源利用率最高"列为加分项。round-robin 在异构请求下容易把每个节点都打散成无法承载大请求的碎片；best-fit 优先消耗已有碎片，保留连续大块容量。

**为什么 duplicate 走 header 而不是 body 字段？** 题面响应字段是固定两个（`node_id`、`remaining_quota`），严格的契约校验器会拒绝多余字段。把 duplicate 标识放 header 既不破坏契约，又给观测/客户端留出旁路信号。

**Store 接口为什么独立？** 题目要求是单进程内存实现，但生产化往往要换分布式存储（Redis、etcd）。把 `Store` 抽出来，business logic 不需要改动就能换实现；同时也让 allocator 单测可以直接 mock store 行为。
