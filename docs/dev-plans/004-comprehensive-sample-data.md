# DEV-PLAN-004：全模块示例数据填充

**状态**: 规划中（2025-11-30 14:15）

## 背景
当前只有核心用户/权限数据，其他模块通过 TestKit 或 UI 操作均无示例记录，无法直观演示 ERP 功能。为提升展示与测试效率，需要构建一套“全模块”种子流程，让 CRM、Finance、HRM、Warehouse、Projects、BI 等关键模块在启动后即时拥有不少于 100 条业务数据（表单、交易、库存、员工等）。

## 目标
- 在单次执行脚本（或 `/__test__/seed` 场景）后，即可在所有启用模块看到可演示的列表。
- 至少生成 100 条具有业务意义的数据：如 20+ 员工、30+ 财务交易、20+ 客户、20+ 仓储项等，覆盖不同状态/类型。
- 数据分布合理：包含多租户字段、货币、语言、时间跨度，支持图表/BI 组件展示。
- 种子流程可重复执行（可清空重建或增量写入），并提供文档说明与验证脚本。

## 实施步骤

1. **数据域梳理与规模规划**  
   - 列出所有启用模块及核心实体：用户、角色、客户、机会/合同、钱账户/支付/费用、员工/岗位、仓储单位/产品/订单、项目/阶段、日志等。  
   - 为每类实体设定数量目标与字段覆盖（不同状态、货币、日期范围），汇总成数据矩阵，确保总体 >100 条记录。

2. **TestKit 场景设计与 JSON 规范**  
   - 扩展 `modules/testkit/services/test_data_service.go` 场景描述，加入 finance/CRM/warehouse 等字段示例；定义引用方式（`@moneyAccounts.mainCash` 等）。  
   - 追加一个“full_demo”或“enterprise”场景，与 `/__test__/seed` API 联动，可清空再填充。

3. **PopulateService 功能补全**  
   - 在 `modules/testkit/services/populate_service.go` 实现 `createMoneyAccounts`、`createPaymentCategories`、`createPayments`、`createCRMData`、`createWarehouseData` 等 TODO，调用各模块仓储/服务并处理引用。  
   - 支持 `Options.ClearExisting`：当为 true 时，清空相关表再写入，保证数据一致性。

4. **数据生成器与脚本**  
   - 若手写 JSON 成本过高，可增加 Go 级数据生成器（例如用 `go generate` 或 CLI）自动构建批量记录。  
   - 输出 `.json` 模板或 `cmd/command/main.go` 子命令（如 `make demo-data`）来触发填充。

5. **验证与质量监控**  
   - 编写自动化检查脚本：验证关键表的记录数、非空字段、referential integrity。  
   - 在文档中列出验证命令（SQL/Go 测试），并在 CI 或手动 checklist 中执行。

6. **文档与演示说明**  
   - 更新 `docs/dev-plans/003`、README、CONTRIBUTING 或独立指南，说明如何启用示例数据、默认账号、数据范围。  
   - 若 Demo 站需长期使用示例数据，提供刷新流程（定时清空重建）。

7. **里程碑与验收**  
   - M1：完成数据矩阵与新场景定义。  
   - M2：补完 PopulateService + 自动脚本，可生成 ≥100 条数据。  
   - M3：通过脚本/SQL 验证、文档交付，并在本地/演示环境验证页面展示。

## 交付物
- 更新后的 TestKit 场景定义与 PopulateService 实现。
- 可重复执行的 demo 数据脚本或 API。
- 验证脚本/SQL 及文档说明，确保任意开发者可以重建完整样例数据。
