# PostgreSQL 17 迁移与备份（本地开发）

本手册用于本地从旧版本 PostgreSQL（例如 15）迁移到 `postgres:17` 的数据备份与恢复。

> 说明：仓库默认本地 compose 端口映射通常为 `5438 -> 5432`（以 `compose.dev.yml` 与 `.env.example` 为准）。

## 1. 备份旧数据（示例：postgres:15）

```bash
docker compose -f compose.dev.yml down db
docker run --rm --network host -v sdk-data:/var/lib/postgresql/data postgres:15 \
  pg_dump -h localhost -p 5438 -U postgres -F c -b -v -f /tmp/pg15-backup.dump iota_erp
```

将 `/tmp/pg15-backup.dump` 复制到安全位置。

## 2. 清理旧卷并切换到 17

```bash
docker volume rm sdk-data
docker compose -f compose.dev.yml up -d db redis
```

## 3. 重新迁移与种子

```bash
DB_HOST=localhost DB_PORT=5438 DB_NAME=iota_erp DB_USER=postgres DB_PASSWORD=postgres make db migrate up
DB_HOST=localhost DB_PORT=5438 DB_NAME=iota_erp DB_USER=postgres DB_PASSWORD=postgres make db seed
```

## 4. 从备份恢复（可选）

使用 `pg_restore` 将备份导入新的 `postgres:17` 实例（按你的实际容器/端口调整参数）。

