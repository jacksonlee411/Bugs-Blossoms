# DEV-PLAN-011：HRM Atlas + Goose PoC 记录

> 目的：记录 011A 补齐后的本地可复现验证步骤（时间 → 命令 → 预期 → 实际 → 结论/回滚）。

## 环境信息
- 日期：2025-12-14
- 机器/OS：Linux
- Go 版本：go1.24.10
- Postgres 版本（本地 compose/CI）：Postgres 17（本地：psql 17.7）
- Atlas 版本：Makefile 固定 `ATLAS_VERSION=v0.38.0`（源码构建安装）
- goose 版本：v3.26.0

## 验证步骤

### 1) 安装工具
- 命令：`make atlas-install && make goose-install`
- 预期：`atlas`/`goose` 可用且版本一致
- 实际：通过

### 2) 启动数据库
- 命令：`docker compose -f compose.dev.yml up -d db`
- 预期：Postgres 17 可连接（默认映射端口 `5438`）
- 实际：通过

### 3) 应用基础迁移（非 HRM）+ HRM 迁移
- 命令：
  - `DB_NAME=iota_erp_hrm_atlas make db migrate up`
  - `DB_NAME=iota_erp_hrm_atlas HRM_MIGRATIONS=1 make db migrate up`
- 预期：基础迁移可执行；HRM baseline + smoke 可执行
- 实际：通过（goose 迁移版本到 `2`）

### 4) Atlas plan/lint
- 命令：
  - `DB_NAME=iota_erp_hrm_atlas ATLAS_DEV_DB_NAME=hrm_dev make db plan`
  - `DB_NAME=iota_erp_hrm_atlas ATLAS_DEV_DB_NAME=hrm_dev make db lint`
- 预期：plan 输出 SQL（不落盘）；lint 通过且无缺文件/缺库类失败
- 实际：通过

### 5) 导出 HRM schema + sqlc 生成
- 命令：`scripts/db/export_hrm_schema.sh SKIP_MIGRATE=1 && make sqlc-generate`
- 预期：`git status --short` 干净
- 实际：通过

## 结论与问题
- 结论：011A 工具链在本地可跑通（Atlas plan/lint + goose migrate + schema export + sqlc generate）。
- 遗留问题：无
- 回滚/清理：删除本地测试数据库可用 `DROP DATABASE ...`（按需）
