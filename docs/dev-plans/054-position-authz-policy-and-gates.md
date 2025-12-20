# DEV-PLAN-054：Position 权限（Authz）与策略门禁（对齐 051 阶段 C-Authz）

**状态**: 草拟中（2025-12-20 01:46 UTC）

## 1. 背景
- 050 §9 定义了 Position/Assignment 相关能力的可见性与授权边界（读/写/生命周期操作/读历史/改历史/读统计/维护分类/维护限制/维护冻结窗口与 reason code）。
- 本计划将其落地为 Casbin 的 object/action、策略碎片与测试门禁，并与 Org API（026）保持一致。

## 2. 工具链与门禁（SSOT 引用）
- Authz 流程与 pack 规则：见 `docs/runbooks/AUTHZ-BOT.md`。
- 门禁触发与本地必跑：见 `AGENTS.md`（Authz 相关：`make authz-test && make authz-lint`，以及策略聚合生成的工作流）。

## 3. 目标（完成标准）
- [ ] object/action 覆盖 050 §9 的能力清单，且与 API/事件字段一致（避免 UI 权限与 API 权限口径漂移）。
- [ ] 策略碎片与聚合产物可复现生成，CI 门禁可通过。
- [ ] 关键越权用例有自动化测试（最小集即可），并可定位到具体拒绝原因。

## 4. 依赖与并行
- 依赖：`DEV-PLAN-052`（合同与能力清单冻结）；`DEV-PLAN-053`（API 入口与对象边界落地）。
- 可并行：UI（DEV-PLAN-055）可先按 object/action 设计进行页面分层与按钮显隐，但最终联调依赖本计划完成。

## 5. 实施步骤
1. [ ] 定义 object/action 矩阵（对齐 050 §9）
   - Position：读、列表、读历史、创建、更新（Update/Correct）、撤销（Rescind）、转移、维护汇报链（reports-to）、冻结窗口相关操作。
   - Assignment：读、占用/释放（FTE）、改历史（若允许）、查询占编/空缺。
   - 主数据与限制（如纳入本计划范围）：Job Catalog/Profile、Restrictions 的维护与只读权限。
   - 统计：组织维度统计与空缺分析的读取权限。
2. [ ] 权限边界与对象归属
   - 对齐“业务从属 OrgUnit、模型独立聚合根”的原则：授权以组织范围与对象归属一致表达。
   - 明确“可改历史（Correct）”与“只读历史”的差异授权，避免高风险能力默认开放。
3. [ ] 策略碎片落地与聚合（遵循 AUTHZ-BOT 工作流）
   - 在 `config/access/policies/**` 中新增/调整策略碎片，并按流程生成聚合产物（避免手改聚合文件）。
4. [ ] 测试与门禁
   - 补齐最小越权用例集（例如：无组织权限读不到 Position；不能 Correct 历史；不能超范围读统计等）。
   - 执行 Authz 门禁并记录命令/结果/时间戳到收口计划指定的 readiness 记录。

## 6. 交付物
- Position/Assignment 的 object/action 定义与策略碎片（含必要的测试用例）。
- 可复现生成的策略聚合产物与门禁记录。

