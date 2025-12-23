# DEV-PLAN-061 示例数据集（seed_061）

本文档用于**记录**并**可重复复现** DEV-PLAN-061（组织-职位-人员-事件）联动链路的开发样例数据集：一次性生成 **20 名员工**（`hire 20 / transfer 3 / termination 1`）+ `D061` 组织节点 + 自动职位/任职（assignment）。

权威业务契约请以 `docs/dev-plans/061-org-position-person-bridge-and-minimal-personnel-events.md` 为准；本文只记录“用于开发/验证的样例数据集”。

## 入口命令

- 运行：`go run ./cmd/command/main.go seed_061`
- 代码实现：`pkg/commands/seed_devplan061.go`

## 前置条件

- Org 迁移已执行（需要 `org_personnel_events` 等表）：
  - `make org migrate up`（必要）
- 默认租户：`00000000-0000-0000-0000-000000000001`
- 自动职位开关（默认开启）：`ENABLE_ORG_AUTO_POSITIONS=true`

## 数据集内容（061）

### 组织节点（D061）

在默认租户下创建（若已存在则复用）：

- `D061-ROOT`：`DEV-PLAN-061 Root`
  - 若租户已有 root 节点：`D061-ROOT` 会作为其子节点创建
  - 若租户还没有 root：`D061-ROOT` 会作为 root 创建（受 Org “只允许一个 root”约束）
- 部门节点（父节点为 `D061-ROOT`）：
  - `D061-ENG`：Engineering
  - `D061-HR`：Human Resources
  - `D061-SALES`：Sales
  - `D061-OPS`：Operations

### 员工（Persons）

创建 20 个 Person（若 `pernr` 已存在则跳过创建，但后续事件仍会写入/复用）：

| pernr | display_name |
| --- | --- |
| 061001 | Ava Reed |
| 061002 | Bruno Silva |
| 061003 | Chloe Tanaka |
| 061004 | Diego Alvarez |
| 061005 | Elena Petrova |
| 061006 | Farah Khan |
| 061007 | Gabriel Martin |
| 061008 | Hana Suzuki |
| 061009 | Ibrahim Noor |
| 061010 | Jia Wei |
| 061011 | Kira Novak |
| 061012 | Liam O'Connor |
| 061013 | Mina Park |
| 061014 | Noah Johnson |
| 061015 | Olivia Rossi |
| 061016 | Pavel Smirnov |
| 061017 | Quinn Murphy |
| 061018 | Rina Sato |
| 061019 | Santiago Perez |
| 061020 | Tara Williams |

### 人事事件（org_personnel_events）

事件通过服务层写入（`modules/org/services/org_personnel_events.go`），并触发最小闭环：

- `hire`：20 条（每人 1 条）
  - `OrgService.HirePersonnelEvent`：写 `org_personnel_events` + 创建 primary assignment（自动职位）+ outbox
- `transfer`：3 条（以下 pernr）
  - `061017`、`061018`、`061019`
  - `OrgService.TransferPersonnelEvent`：写事件 + 更新 primary assignment（新的自动职位）+ outbox
- `termination`：1 条
  - `061020`
  - `OrgService.TerminationPersonnelEvent`：写事件 + rescind primary assignment + outbox

#### 有效日期（effective_date）

为避免 Org 冻结窗口（freeze window）对历史月份的限制，seed 采用：

- `baseEffective = 当前 UTC 月的月初（YYYY-MM-01T00:00:00Z）`
- `hire`：`baseEffective + i 天`（i 从 0 到 19）
- `transfer`：`baseEffective + 45 天`
- `termination`：`baseEffective + 75 天`

#### request_id 约定（可重复执行）

为了可重复执行（idempotent-ish），请求号固定为：

- `seed_061:node:<code>`
- `seed_061:hire:<pernr>`
- `seed_061:transfer:<pernr>`
- `seed_061:termination:<pernr>`

`org_personnel_events` 以 `(tenant_id, request_id)` 唯一约束去重；重复运行不会重复插入同一 `request_id` 的事件。

### 自动职位/任职（org_positions / org_assignments）

在 `hire/transfer` 时使用 `PositionID=nil`，由 Org 自动职位逻辑生成：

- `CreateAssignment`/`UpdateAssignment` 会基于 `autoPositionID(tenant_id, org_node_id, subject_id)` 生成 `org_positions`（`is_auto_created=true`）与 `org_position_slices`
- `hire` 会创建 primary assignment
- `transfer` 会在新 org_node 下生成新的自动职位并切换 primary assignment
- `termination` 会 rescind primary assignment

## 验证方法

### SQL 验证（示例）

```
select count(*) from persons where pernr like '061%';
select event_type, count(*) from org_personnel_events where pernr like '061%' group by event_type order by event_type;
select count(*) from org_nodes where code like 'D061-%';
```

期望（仅针对本数据集的增量）：`hire=20 / transfer=3 / termination=1`。

### UI 验证（示例）

- 登录后访问：
  - `http://localhost:3200/person/persons`
  - `http://localhost:3200/org/nodes`

## 注意事项

- 本 seed **不会清理**既有数据：仅做“创建/写入”，并尽量通过唯一约束实现可重复执行。
- 若本地尚未启用 Org rollout，`/org/nodes` 可能被 404；开发环境建议在 `.env.local` 配置：
  - `ORG_ROLLOUT_MODE=enabled`
  - `ORG_ROLLOUT_TENANTS=00000000-0000-0000-0000-000000000001`

