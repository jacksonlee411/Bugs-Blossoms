# DEV-PLAN-024：Org 主链 CRUD（Person→Position→Org）

**状态**: 规划中（2025-01-15 12:00 UTC）

## 背景
- 对应 020 步骤 4，承接 021（schema 与约束）与 022（占位表/事件契约）、023（导入/回滚），需交付 Person→Position→Org 单树的核心 CRUD，含自动创建空壳 Position，全链路强制 Session+租户隔离。

## 目标
- Person→Position→Org 单树 CRUD 可用且通过租户/Session 校验。
- 自动创建一对一空壳 Position 的逻辑生效。
- M1 仅开放 primary assignment 写入，matrix/dotted 保持只读占位或特性开关关闭。
- 全链路遵守 DDD 分层/cleanarch 约束，禁止跨层耦合。
- 所有接口/服务接受 `effective_date` 参数，缺省按 `time.Now()` 处理。

## 范围与非目标
- 范围：OrgNode/OrgEdge/Position/OrgAssignment 主链 CRUD，自动创建一对一空壳 Position，Session+租户强校验，primary assignment 写入与事件发布，基础树/表单页面与文档说明。
- 非目标：不开放 matrix/dotted assignment 写入（仅只读或特性开关关闭）、不实现审批/草稿/仿真/retro/Impact、不开启角色/继承业务逻辑执行（沿用 022 占位）。

## 依赖与里程碑
- 依赖：021 schema 约束（单树、无重叠/无环）、022 事件契约（OrgChanged/OrgAssignmentChanged）、023 导入/回滚脚本与对账；统一使用 023 定义的 CSV 模板格式与批次 token。
- 里程碑（按提交时间填充）：service/repo CRUD -> controller/DTO/mapper + Session/tenant 校验 -> 自动 Position + assignment 约束 + 事件 -> templ 页面与文档 -> 测试与 readiness 记录。

## 设计决策
- 架构与权限：遵守 DDD/cleanarch，controller 负责 Session+租户校验（无 Session/tenant 直接 401/403），service 接口接受租户上下文，repo 强制 tenant 过滤，禁止跨层耦合。
- 有效期与校验：所有写入接受 `effective_date`/`end_date`，缺省 `time.Now()`/`9999-12-31` 半开区间，复用 021 的无重叠/无环/父子一致校验；冻结窗口沿用 020 默认（月末+3 天，可租户覆盖）。
- CRUD 行为：OrgNode/OrgEdge/Position/OrgAssignment 提供 Create/Update/Correct（原位更正需更高权限）/Rescind，默认 Update 截断旧时间片；Position 必须绑定 OrgNode，OrgAssignment 必须绑定 Position。
- 自动 Position：当创建 Assignment 未显式提供 position_id 时，自动生成一对一空壳 Position（标记 is_auto_created），并绑定 OrgNode；重复请求以幂等键避免重复创建。
- Assignment 范围：仅允许 primary 写入，matrix/dotted 通过特性开关默认关闭；同一 subject+有效期内 primary 唯一，重叠拒绝。
- HRM 软校验：person_id/pernr 仅软校验（跨 SOR），失败提示但不阻塞；position_id 归属 OrgNode 必须强校验。
- 事件与 outbox：所有写操作成功后发布 `OrgChanged` / `OrgAssignmentChanged` 事件（对齐 022 契约，含 event_id/event_version/sequence/effective_window/assignment_type），outbox 与事务一致。
- API/模板：REST/HTMX 控制器接受 `effective_date` 查询点，返回显式值与继承解析值占位；基础树/表单 templ 页面提供节点/Position/Assignment 创建编辑，遵守 `templ generate && make css` 生成流程。
- 权限：接口按 020 最小集 `Org.Read/Org.Write/Org.Assign/Org.Admin` 判定，拒绝未授权访问；matrix/dotted 的写入口默认关闭。

## 任务清单与验收标准
1. [ ] service/repo CRUD：实现 OrgNode/OrgEdge/Position/OrgAssignment 的 Create/Update/Correct/Rescind，repo 强制 tenant 过滤；支持 `effective_date` 默认值、无重叠/无环校验、OrgEdge 与 OrgNode 一致校验。验收：通过单元/集成测试验证租户隔离与有效期校验。
2. [ ] 自动 Position 与 Assignment 约束：在 Assignment 写入缺少 position_id 时自动创建空壳 Position 并绑定 OrgNode，幂等防重复；primary 唯一、matrix/dotted 写入拒绝或由特性开关保护。验收：测试覆盖有/无 position_id、重复请求、matrix/dotted 被拒绝场景。
3. [ ] Controller/DTO/Mapper：实现 REST/HTMX 控制器与 DTO，强制 Session+租户校验，支持 `effective_date` 查询，按权限返回/拒绝；区分 Update 与 Correct，返回明确信息。验收：接口级测试覆盖无 Session/无租户/权限不足/有效请求。
4. [ ] 事件发布与 outbox：对接 `OrgChanged` / `OrgAssignmentChanged` 契约，写操作生成事件并写入 outbox，含幂等键与 assignment_type。验收：事件 payload 字段与 022 契约对齐，测试验证事件生成与幂等键。
5. [ ] templ 页面与文档：提供基础树/表单 templ 页面（节点/Position/Assignment 创建编辑），更新模块文档/README，执行 `templ generate && make css`。验收：生成后 `git status --short` 干净。
6. [ ] Readiness：执行 `make check lint`、`go test ./modules/org/...`（或影响路径），必要时 `make check tr` 如有文案变更；将命令/耗时/结果记录到 `docs/dev-records/DEV-PLAN-024-READINESS.md`。

## 验证记录
- 将测试/生成/检查命令与结果写入 `docs/dev-records/DEV-PLAN-024-READINESS.md`，若有对账或临时文件需在文档中引用，确认 `git status --short` 干净。

## 风险与回滚/降级路径
- 业务风险：自动 Position 可能重复生成，需幂等键保护并允许回滚；若冲突，提供开关关闭自动创建并要求显式 position_id。
- 兼容风险：matrix/dotted 默认关闭，如未来打开需增加权限/验证与事件版本演进。
- 发布回滚：若 CRUD 写入导致约束冲突，可回滚至导入前快照或使用 023 rollback 脚本清理批次；如需撤销迁移，使用 org 迁移目录的 `make db migrate down`，避免影响 HRM。

## 交付物
- 主链 CRUD 代码与测试（domain/service/repo/controller/mapper）。
- 事件发布与自动 Position 逻辑。
- 基础 templ 页面及生成产物（生成后工作区干净）。
- 文档与验证记录（`docs/dev-records/DEV-PLAN-024-READINESS.md`）。
