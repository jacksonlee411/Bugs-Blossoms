# DEV-PLAN-072A：Job Catalog 二级菜单中文名修正为“职位分类” + 职种创建补齐

**状态**: 草拟中（2025-12-30 15:04 UTC）

## 1. 背景与上下文 (Context)
- **现状**：左侧导航 `组织与职位` 下二级菜单 `Job Catalog` 在中文被翻译为“职位模板”，与页面实际内容（职类/职种/职级/职位模板四类主数据维护）不匹配，造成用户把“菜单名=职位模板（Job Profile）”误解为仅管理模板。
- **直接影响**：在 `职位分类`（Job Catalog）页面的 `职种`（Job Family）页签下，当未选择 `职类`（Job Family Group）时页面展示“未找到”，用户误以为“职种没有创建功能”。

## 2. 目标与非目标 (Goals & Non-Goals)
### 2.1 目标
- [ ] 将 `Job Catalog` 二级菜单中文名统一为“职位分类”，并同步页面 `MetaTitle/Title`，避免“菜单名 ≠ 页面标题”。
- [ ] 明确术语边界：`职位模板=Job Profile`（页签/字段/概念保持不变），`职位分类=Job Catalog`（入口/页面）。
- [ ] 让“职种（Job Family）”在职位分类页面可直接创建：进入 `tab=families` 时默认选中一个职类（优先启用项），确保创建表单可见。
- [ ] 调查并修正文档中把该二级菜单称为“职位模板”的表述，给出更新建议并落地最小修正，避免 SSOT 漂移。

### 2.2 非目标
- 不调整 `Job Profile=职位模板` 的术语决策（见 `DEV-PLAN-072`）。
- 不改动 Job Catalog / Job Profile 的数据模型与 API 合同形状（本计划仅做“文案/默认选择/可见性”修正）。
- 不新增数据库表/迁移。

## 2.3 工具链与门禁（SSOT 引用）
- 触发器矩阵与本地必跑：`AGENTS.md`
- 命令入口：`Makefile`
- CI 门禁：`.github/workflows/quality-gates.yml`

## 3. 方案 (Proposal)
### 3.1 i18n/导航：将“Job Catalog”入口中文名调整为“职位分类”
- 修改 `modules/org/presentation/locales/zh.json`：
  - `NavigationLinks.JobCatalog`：从“职位模板”改为“职位分类”
  - `Org.UI.JobCatalog.Title` / `Org.UI.JobCatalog.MetaTitle`：从“职位模板”改为“职位分类”
  - 保持 `Org.UI.JobCatalog.Tabs.Profiles` 为“职位模板”（这是 `Job Profile` 主数据对象的中文名）

### 3.2 UX：补齐“职种”创建的可见性（默认选中职类）
- 服务端默认：当 `GET /org/job-catalog?tab=families` 且未提供 `job_family_group_code` 时：
  - 若存在职类：自动选择一个职类（优先 `is_active=true` 的第一项，否则第一项），渲染职种列表与 Create 表单。
  - 若不存在职类：维持现有提示（必要时再增强为“请先创建职类”）。

### 3.3 文档调查与更新建议（误把菜单称为“职位模板”）
- 已定位需要修正的文档：
  - `docs/dev-plans/072-job-architecture-workday-alignment.md`：5.3 节标题与“UI 入口”描述
  - `docs/dev-plans/056-job-catalog-profile-and-position-restrictions.md`：对齐更新备注
- 更新建议：
  - 用“职位分类（Job Catalog）”指代入口/页面；用“职位模板（Job Profile）”指代 Profiles 页签与主数据对象。
  - 在 `DEV-PLAN-072` 引用 `DEV-PLAN-072A` 作为命名修正补丁，避免读者误解“入口=职位模板”。

## 4. 实施步骤 (Steps)
1. [ ] 文档：新增本计划，并在 `AGENTS.md` Doc Map 挂链接
2. [ ] i18n：更新 `modules/org/presentation/locales/zh.json` 的菜单与页面标题
3. [ ] UI：实现 families 默认职类选择，确保“职种创建”表单可见
4. [ ] 文档：更新 `DEV-PLAN-072` / `DEV-PLAN-056` 的相关描述
5. [ ] 验证：按 `AGENTS.md` 触发器执行 `make check tr`、`make check doc`，以及 Go 相关门禁

## 5. 验收标准 (Acceptance)
- 左侧导航二级菜单显示“职位分类”，页面标题/MetaTitle 一致。
- `Profiles` 页签仍显示“职位模板”。
- 打开 `/org/job-catalog?tab=families` 在已有职类时直接看到职种列表与创建表单；可成功创建职种并回到 families 列表。
