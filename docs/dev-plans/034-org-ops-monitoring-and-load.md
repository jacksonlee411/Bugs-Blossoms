# DEV-PLAN-034：Org 运维治理与压测（Step 14）

**状态**: 已评审（2025-12-18 12:00 UTC）— 按 `docs/dev-plans/001-technical-design-template.md` 补齐可编码契约

## 0. 进度速记
- 本计划交付“可观测性（metrics/health）+ 可重复压测 + 运维脚本入口”，用于支撑 Org 长期运行与灰度/回滚演练。
- 线上热点查询禁止递归：递归 CTE 仅允许在离线 build/压测数据构建中使用；在线读路径必须复用 029 的 deep-read 后端选择。

## 1. 背景与上下文 (Context)
- **需求来源**：`docs/dev-plans/020-organization-lifecycle.md` **步骤 14：运维、治理与压测**。
- **依赖链路**：
  - `docs/dev-plans/017-transactional-outbox.md` + `docs/runbooks/transactional-outbox.md`：outbox/relay/cleaner 的指标与排障口径（SSOT）。
  - `docs/dev-plans/018B-routing-strategy-gates.md`：ops 暴露基线（`/health`、`/debug/prometheus` 必须受保护）。
  - `docs/dev-plans/027-org-performance-and-rollout.md`：性能预算、灰度与回滚剧本、query budget 守卫（上线门槛）。
  - `docs/dev-plans/029-org-closure-and-deep-read-optimization.md`：closure/snapshot build 与回滚（深读/报表的性能底座）。
  - `docs/dev-plans/031-org-data-quality-and-fixes.md`：质量报告/修复与回滚 manifest（数据治理闭环）。
  - `docs/dev-plans/033-org-visualization-and-reporting.md`：导出/路径/BI 报表（依赖稳定可观测性与压测基线）。
- **当前痛点**：
  - 缺少 Org 领域级指标（读/写耗时、冲突、缓存命中、deep-read build 耗时），排障只能靠日志与手工 SQL。
  - 缺少可重复的压测入口与基线，无法识别性能回退、缓存失效不一致、outbox 堆积等运行期问题。
  - 运维动作（缓存重建、快照刷新、回滚）在多个计划中分散，需要“统一入口 + 清晰的安全网”。

## 2. 目标与非目标 (Goals & Non-Goals)
- **核心目标**：
  - [ ] **指标（Prometheus）**：定义并落地 Org 领域指标（见 §4.1），覆盖：
    - API 维度（请求量/错误率/延迟）
    - DB 维度（关键写冲突/重试、query budget 守卫）
    - Cache 维度（命中率、失效次数）
    - Outbox 维度（复用 017/`pkg/outbox` 指标，确保 org_outbox 纳入）
    - Deep-read build（029 closure/snapshot build 时长、行数、成功/失败）
  - [ ] **健康/就绪检查**：
    - [ ] 全局 `/health`（既有）保持轻量、快速、生产受保护（OpsGuard）。
    - [ ] 新增 Org 级 `/org/api/ops/health`（见 §5.3）：输出“可执行的排障线索”（outbox backlog、snapshot build freshness、cache 状态），并要求 `org.ops admin`。
  - [ ] **自动化压测（可重复）**：
    - [ ] 提供 `org-load`（Go CLI）或等价脚本入口（见 §5.4），覆盖读/写混合场景，输出 `org_load_report.v1`（见 §4.3）。
    - [ ] 固化最小压测 profile（1k/10k 节点、混合读写），并定义阈值/失败判定（见 §9）。
  - [ ] **运维脚本入口（统一）**：
    - [ ] 复用并收敛已有能力：导入/回滚（023）、灰度/回滚（027）、snapshot/closure refresh/activate/prune（029）、质量修复（031）。
    - [ ] 以 `Makefile` 入口为准，避免“脚本散落且无法追溯”。
  - [ ] Readiness：新增 `docs/dev-records/DEV-PLAN-034-READINESS.md`（实现阶段落盘），记录门禁命令、压测结果摘要与关键指标截图/采样。
- **非目标 (Out of Scope)**：
  - 不在本计划内建设完整 SRE 体系（值班轮值、跨环境告警模板库、Grafana 作为代码等），只提供最小可运行基线。
  - 不引入重量级外部压测工具链（如 k6/locust）作为硬依赖；优先使用 Go CLI 以减少环境漂移（如需引入另起评审）。
  - 不在本计划内实现策略生成（Casbin policy draft 仍以 015A 为 SSOT）。

### 2.1 工具链与门禁（SSOT 引用）
> 本计划会新增 Go 代码、路由、Authz、可能新增迁移；命令细节以 SSOT 为准，本文不复制矩阵。

- **命中触发器（`[X]` 表示本计划涉及该类变更）**：
  - [X] Go 代码（metrics/health/load runner/测试）
  - [X] 路由治理（新增 `/org/api/ops/health`，以及可选 `/org/api/ops/*`）
  - [X] Authz（新增 `org.ops` object/action 映射与策略片段）
  - [ ] 迁移 / Schema（仅当需落地 ops 记录表/报表快照表时触发；如触发必须走 021A）
  - [X] 文档 / Readiness（新增 034 readiness record）
  - [ ] `.templ` / Tailwind、多语言 JSON（本计划不涉及）

- **SSOT（命令与门禁以这些为准）**：
  - `AGENTS.md`
  - `Makefile`
  - `.github/workflows/quality-gates.yml`
  - `docs/dev-plans/009A-r200-tooling-playbook.md`
  - `docs/dev-plans/021A-org-atlas-goose-toolchain-and-gates.md`
  - `docs/runbooks/transactional-outbox.md`（outbox 运维/指标/排障）

### 2.2 与其他子计划的边界（必须保持清晰）
- 017：outbox 指标与排障口径 SSOT；034 只确保“org_outbox 被纳入 relay/metrics”与告警阈值建议。
- 027：性能预算与灰度回滚剧本 SSOT；034 的压测/告警阈值不得与 027 冲突（预算升级需附基准支持）。
- 029：deep-read build 与回滚 SSOT；034 只补充“build 观测/健康/压测”与运维入口。
- 031：质量修复与回滚 manifest SSOT；034 不重写修复逻辑，只把其纳入 ops 剧本与观测。
- 018B：ops 暴露基线 SSOT；`/health` 与 `/debug/prometheus` 保护规则以 `pkg/middleware/ops_guard.go` 为准。

## 3. 架构与关键决策 (Architecture & Decisions)
### 3.1 架构图 (Mermaid)
```mermaid
flowchart TD
  App[Server] -->|/debug/prometheus| Prom[Prometheus Scrape]
  App -->|/health| Health[/health/]
  App -->|/org/api/ops/health| OrgHealth[/org/api/ops/health/]

  App --> M[Org Metrics Instrumentation]
  App --> O[Outbox Relay/Cleaner Metrics (017)]

  Load[org-load CLI] -->|HTTP| App
  Load -->|JSON report| Report[org_load_report.v1.json]
```

### 3.2 关键设计决策（ADR 摘要）
1. **Prometheus 指标命名与低基数（选定）**
   - 指标必须避免 `tenant_id/org_node_id/pernr` 等高基数 label；使用固定枚举 label（endpoint/op/result/backend）。
2. **/health 保持轻量（选定）**
   - 全局 `/health` 仅做“快速存活性”检查；Org 级健康/新鲜度/堆积等放在 `org.ops health`（内部 API）中输出。
3. **压测工具优先 Go CLI（选定）**
   - 使用 Go CLI 以保证本地/CI 可重复；输出固定 JSON，便于对比与门禁扩展。

## 4. 数据契约 (Data Contracts)
> 本节定义指标与压测报告的 SSOT；outbox 指标见 `pkg/outbox/metrics.go` 与 runbook。

### 4.1 Prometheus 指标（Org v1，选定）
> 说明：本节是“指标名/label 的契约”。实现可集中在 Org controller/service middleware，不要求一次性覆盖全部端点，但必须先覆盖 024/026 的关键读写路径。

**API**
- `org_api_requests_total{endpoint,result}`（counter）
  - `endpoint`：固定枚举（例如 `hierarchies.get`, `nodes.create`, `batch.post`）
  - `result`：`2xx|4xx|5xx`
- `org_api_latency_seconds{endpoint,result}`（histogram）

**写冲突/重试（最小版）**
- `org_write_conflicts_total{kind}`（counter）
  - `kind`：`overlap|unique|foreign_key|other`

**缓存（最小版）**
- `org_cache_requests_total{cache,result}`（counter）
  - `cache`：`hierarchy|assignments|snapshot`
  - `result`：`hit|miss`
- `org_cache_invalidate_total{reason}`（counter）
  - `reason`：`write_commit|outbox_event|manual`

**deep-read build（029）**
- `org_deep_read_build_total{type,result}`（counter）
  - `type`：`closure|snapshot`
  - `result`：`ok|failed`
- `org_deep_read_build_latency_seconds{type,result}`（histogram）
- `org_deep_read_active_backend{backend}`（gauge，值恒为 1）
  - `backend`：`edges|closure|snapshot`

**Outbox（017，复用）**
- 由 `pkg/outbox` 提供（示例）：`outbox_pending{table}`、`outbox_dispatch_total{table,topic,result}`、`outbox_dispatch_latency_seconds{...}`。
- 约束：当启用 Org outbox 时，`OUTBOX_RELAY_TABLES` 必须包含 `public.org_outbox`（口径见 026/017）。

### 4.2 Org Ops Health（内部 API，v1）
> 复用 `modules/core/presentation/controllers/health_controller.go` 的响应形状：顶层 `status/timestamp/checks`；Org 只扩展 `checks` 的 key。

新增 checks（建议）：
- `checks.outbox`：pending/locked/oldest_available_age（以 DB 快速聚合查询实现；超阈值 degraded）。
- `checks.deep_read`：active backend、active build age（如启用 029；build 过期 degraded）。
- `checks.cache`：是否启用、最近失效次数采样（如可得）。

### 4.3 `org_load_report.v1`（压测报告）
- 文件名建议：`org_load_report_<profile>_<run_id>.json`

**Schema（v1）**
```json
{
  "schema_version": 1,
  "run_id": "uuid",
  "started_at": "2025-03-01T12:00:00Z",
  "finished_at": "2025-03-01T12:05:00Z",
  "target": { "base_url": "http://localhost:3200", "tenant_id": "uuid" },
  "profile": { "name": "org_read_1k", "vus": 20, "duration_seconds": 120 },
  "backend": { "deep_read_backend": "edges|snapshot|closure", "cache_enabled": true },
  "results": [
    { "endpoint": "hierarchies.get", "count": 1000, "errors": 0, "p50_ms": 10, "p95_ms": 50, "p99_ms": 120 }
  ],
  "thresholds": [{ "name": "p99_ms", "limit": 200, "ok": true }],
  "notes": ""
}
```

## 5. 接口契约 (API / CLI Contracts)
### 5.1 `/health`（全局）
- 路径与响应：见 `modules/core/presentation/controllers/health_controller.go`（已有）。
- 生产暴露基线：必须受 OpsGuard/网关 allowlist/BasicAuth 至少一种保护（见 018B 与 `pkg/middleware/ops_guard.go`）。

### 5.2 `/debug/prometheus`（全局）
- 路径：默认 `/debug/prometheus`（可由 `PROMETHEUS_METRICS_PATH` 配置）。
- 生产暴露基线：同 `/health`（见 018B 与 OpsGuard）。

### 5.3 `GET /org/api/ops/health`
> Org 级健康/新鲜度/堆积检查；返回 200/503 与 `status` 一致；403 payload 对齐 026。

**Query**
- `effective_date`：可选（缺省 `nowUTC`）；用于 deep-read freshness 判断（可选）。

**Response 200/503**
```json
{
  "status": "healthy",
  "timestamp": "2025-03-01T12:00:00Z",
  "checks": {
    "database": { "status": "healthy", "responseTime": "10ms" },
    "outbox": { "status": "healthy", "details": { "pending": 0, "locked": 0 } },
    "deep_read": { "status": "degraded", "details": { "backend": "snapshot", "active_build_age": "48h" } }
  }
}
```

**Authz**
- object：`org.ops`
- action：`admin`

### 5.4 压测入口（CLI / Make）
> 最终以 `Makefile` 为 SSOT；CLI 只是实现形态。要求：输出 `org_load_report.v1`，并支持固定 profile。

- 建议二进制：`org-load`
- 子命令（契约）：
  - `org-load run --profile <name> --base-url <url> --tenant <uuid> --sid <cookie> --out <path>`
  - `org-load smoke --base-url <url> --tenant <uuid> --sid <cookie>`（小流量冒烟）
- profile（v1 最小集）：
  - `org_read_1k`：对齐 027 的 1k 节点读预算
  - `org_read_10k`：深读/导出/路径的压力基线
  - `org_mix_read_write`：混合读写（含 `/org/api/batch` dry-run）

## 6. 核心逻辑与算法 (Business Logic & Algorithms)
### 6.1 Org Ops Health 判定（v1）
- **Down**：
  - DB 不可达（无法 `SELECT 1`）；
  - 或 outbox 表存在大量 locked 且超 TTL（疑似 relay 卡死），且持续超过阈值（实现可先做 degraded）。
- **Degraded**（示例阈值，可在实现中配置化）：
  - `outbox_pending{public.org_outbox} > 1000` 或 oldest_available_age > 5m
  - active snapshot build age > 24h（在启用 snapshot backend 时）
  - cache 关闭但 deep-read backend=edges 且导出/路径压力场景下 p99 超预算（可通过压测守卫判定）

### 6.2 指标埋点策略（v1）
- controller 层对每个 endpoint 固定 `endpoint` label（手工常量），避免按 URL path 产生高基数。
- 错误分类：
  - `result=4xx` 包含 401/403/404/422 等业务错误；
  - `result=5xx` 表示服务端错误或超时。

### 6.3 压测执行（v1）
- `org-load` 在运行前先做 health/smoke 校验（例如 `GET /health` 200）。
- 压测请求必须携带 `X-Request-Id`（便于日志关联），并固定 `effective_date` 参数以减少抖动。
- 输出报告必须包含 profile、后端（deep read backend、cache enabled）与阈值判定结果。

## 7. 安全与鉴权 (Security & Authz)
- **Ops 端点保护（全局）**：
  - `/health` 与 `/debug/prometheus` 在生产默认受 `pkg/middleware/ops_guard.go` 保护（route class=ops）；可通过 CIDR/token/basic auth 放行（配置见 `pkg/configuration/environment.go`）。
- **Org ops 健康端点**：
  - 必须走 026 的 `ensureAuthz` 与 forbidden payload；默认只允许 `org.ops admin`。
- **指标 label 红线**：
  - 禁止将 tenant_id、user_id、org_node_id、pernr 作为 Prometheus label（避免 cardinality 爆炸与隐私泄露）。

## 8. 依赖与里程碑 (Dependencies & Milestones)
- **依赖**：
  - 026：Authz/403 payload 与 `/org/api/*` 的统一约束。
  - 027：性能预算与灰度回滚剧本（压测阈值对齐）。
  - 029：deep-read build 与回滚（健康检查与压测后端选择依赖）。
  - 017：outbox 指标与排障口径。
- **里程碑**：
  1. [ ] 指标 v1 落地（Prometheus 可 scrape，Org 关键端点覆盖）。
  2. [ ] `GET /org/api/ops/health` 落地（含 outbox/deep-read freshness）。
  3. [ ] `org-load` v1 落地（固定 profile + JSON report + 阈值判定）。
  4. [ ] `docs/dev-records/DEV-PLAN-034-READINESS.md` 补齐（门禁+压测结果+排障演练）。

## 9. 测试与验收标准 (Acceptance Criteria)
- **可观测性**：
  - Prometheus 端点启用后可看到 outbox 指标（至少 `outbox_pending/outbox_dispatch_total`）与 Org v1 指标（至少 `org_api_requests_total/org_api_latency_seconds`）。
  - `/org/api/ops/health` 在 outbox backlog / snapshot build 过期时能返回 `degraded` 或 `down`，并携带可定位细节。
- **压测可重复**：
  - 同一 profile 在同一环境重复运行 3 次，p99 波动 ≤ 20%（记录在 readiness）。
  - `org_read_1k` 的核心读路径 p99 不高于 027 的预算（以本地/CI 基准环境为准）。
- **工程门禁**：
  - 文档：`make check doc` 通过。
  - 如新增 Go/Authz/路由/迁移：按 `AGENTS.md` 触发器矩阵执行并通过。

## 10. 运维与回滚 (Ops & Rollback)
- **排障入口（优先引用 SSOT）**：
  - Outbox 堆积/重试/死信：`docs/runbooks/transactional-outbox.md`
  - 灰度/回滚（租户开关/缓存/读策略）：`docs/dev-plans/027-org-performance-and-rollout.md`
  - deep-read build 刷新/切换/回滚：`docs/dev-plans/029-org-closure-and-deep-read-optimization.md`
  - 数据质量报告/修复/回滚：`docs/dev-plans/031-org-data-quality-and-fixes.md`
- **回滚原则**：
  - 先回滚“开关/后端选择”（正确性优先），再处理数据回滚（如导入/修复），最后才考虑 schema 回滚。

## 11. 交付物 (Deliverables)
- Org v1 Prometheus 指标与 `/org/api/ops/health`。
- `org-load`（或等价）压测入口与 `org_load_report.v1`。
- `docs/dev-records/DEV-PLAN-034-READINESS.md`（实现阶段落盘）。
