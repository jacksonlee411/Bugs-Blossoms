# DEV-PLAN-042：移除 ru/uz 多语言，仅保留 en/zh

**状态**: 规划中（2025-12-23 04:22 UTC）

## 背景
当前仓库支持多语言（含 `ru`/`uz`/`en`/`zh`），但项目实际需求仅保留 `en` 与 `zh`。继续维护 `ru/uz` 会带来：
- 翻译键一致性门禁成本（`make check tr`）与变更阻力
- UI 语言选择项与用户资料语言字段的“无效选项”问题
- Bundle 体积与维护面增大

本计划目标是以“契约优先”的方式，明确移除范围、实现步骤、门禁与回滚策略。

## 目标与非目标

### 目标（Goals）
- [ ] 系统仅暴露并支持 `en`、`zh` 两种语言（UI 选择、cookie/参数解析、用户资料语言等）。
- [ ] 移除 `ru`、`uz` 的 locale 资源与语言枚举，保证 `make check tr` 可通过。
- [ ] 默认语言回退策略清晰：任何非 `en/zh` 的输入最终落到 `en`（或按既有规则落到 `zh`，需明确）。
- [ ] CI 门禁通过（至少包含：Go、翻译一致性、文档门禁）。

### 非目标（Non-Goals）
- 不对既有 `ru/uz` 用户语言数据做数据库迁移/清洗（如需要，另起数据迁移计划）。
- 不对翻译内容质量做补全（仅做语言集合的裁剪与行为一致性）。

## 工具链与门禁（SSOT 引用）
- 本计划命中触发器：
  - [ ] Go 代码（`go fmt ./... && go vet ./... && make check lint && make test`）
  - [ ] 多语言 JSON（`make check tr`）
  - [ ] 文档新增/整理（`make check doc`）
- SSOT：
  - 触发器矩阵：`AGENTS.md`
  - 命令入口：`Makefile`
  - CI 门禁：`.github/workflows/quality-gates.yml`

## 现状盘点（Scope）

### 语言源头（候选改动点）
- 语言枚举/展示：`pkg/intl/intl.go`（`allSupportedLanguages`）
- i18n 中间件语言解析：`pkg/middleware/i18n.go`
- 账号设置语言选择：`modules/core/presentation/controllers/account_controller.go`
- 翻译一致性检查：`pkg/commands/check_tr_keys.go`（Bundle 全量 locale 的一致性校验）
- 各模块 locales 资源：`modules/**/presentation/locales/*.json`（目前普遍包含 `en/ru/uz/zh`）

### 预期裁剪范围
- 删除（或停止加载）所有 `ru.json` / `uz.json` locale 文件（包括 core/person/org/logging/website/testkit 等模块）。
- 禁止 UI/接口暴露 `ru/uz` 语言选项。
- 对外输入（Accept-Language / cookie / query / user profile）若为 `ru/uz`：统一回退到 `en`（或 `zh`，需在实现中固定为一个策略并写入文档）。

## 实施步骤

1. [ ] 设计与决策冻结（本文）
   - [ ] 明确默认回退语言：建议 `en`。
   - [ ] 明确用户资料 `language` 字段遇到 `ru/uz` 的处理：仅“显示/选择”层面回退，还是同时在保存时规范化为 `en/zh`。

2. [ ] 裁剪语言枚举与 UI 选择
   - [ ] 在 `pkg/intl/intl.go` 中移除 `ru/uz`（或引入 whitelist 强制仅 `en/zh`，避免其他模块依赖 `SupportedLanguages` 的“全量默认”）。
   - [ ] 账号设置页仅渲染 `en/zh` 选项，并在保存时拒绝/回退 `ru/uz`。

3. [ ] 调整 i18n 解析与回退策略
   - [ ] `pkg/middleware/i18n.go` 仅接受 `en/zh`，其他语言统一回退到 `en`。
   - [ ] 明确并测试：`Accept-Language=ru`、`lang=uz`、用户资料语言为 `ru` 等场景的最终渲染语言。

4. [ ] 清理 locales 资源并修复门禁
   - [ ] 删除 `modules/**/presentation/locales/ru.json` 与 `modules/**/presentation/locales/uz.json`。
   - [ ] 调整 `make check tr`（通过 `check_tr_keys` 的 allowedLanguages 白名单或 Bundle 仅加载 en/zh），确保门禁不再要求 ru/uz 键一致。

5. [ ] 回归与验收
   - [ ] `make check tr` 通过。
   - [ ] `make check lint && make test` 通过。
   - [ ] UI 验证：登录页/Persons/Org 等核心页面在 `en`、`zh` 下都能正常渲染；不存在 `message not found` 的 panic。

## 风险与回滚

### 风险
- 直接删除 `ru/uz` locales 可能导致：
  - 用户资料中仍存 `ru/uz` 时，本地化逻辑在某些路径仍尝试创建 `ru/uz` Localizer 造成缺失 key（需确保回退逻辑在 Localizer 创建之前生效）。
  - 某些页面使用未命名空间的通用 key（如 `View`/`Edit`）依赖 core locales；必须保证 en/zh 都存在。

### 回滚策略
- 代码回滚：对该计划的 PR 使用 `git revert` 回滚提交。
- 数据回滚：本计划不涉及 DB 迁移；仅需恢复 locales 与语言枚举即可。

## 交付物
- [ ] `docs/dev-plans/042-remove-ru-uz-locales.md`
- [ ] 对应 PR（包含代码与 locales 删除）
- [ ] （如需要）Readiness 记录：`docs/dev-records/DEV-PLAN-042-READINESS.md`

