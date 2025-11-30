# DEV-PLAN-003：数据库初始化与登录修复

**状态**: 已完成（2025-11-30 13:55）

## 背景
浏览器访问 `http://localhost:3200/login` 并使用默认账号 `test@gmail.com / TestPass123!` 登录时，后端日志出现 `ERROR: relation "users" does not exist (SQLSTATE 42P01)`，说明尚未执行迁移与种子，`users` 等核心表缺失，引发“发生内部服务器错误：<no value>”。同时，`modules/core/presentation/assets/css/main.min.css` 未生成，导致登录页静态资源请求返回 404，界面样式异常。需要建立一套恢复流程，确保数据库和前端构建一次性就绪。

## 实施步骤

1. [X] **运行数据库迁移**  
   - 命令：`make db migrate up`  
   - 结果：PostgreSQL 应用 24 条迁移，创建 `users`、`sessions`、`tenants` 等核心表；日志中确认 “Applied 24 migrations”。

2. [X] **执行基础数据种子**  
   - 命令：`make db seed`  
   - 内容：写入默认租户、69 条权限、Admin 角色，以及 `test@gmail.com` 与 `ai@llm.com` 等示例账号，同时同步货币和 AI Chat 配置，保证认证与演示数据齐备。

3. [X] **编译 Tailwind CSS**  
   - 命令：`make css`（若已有 watcher 运行，仍手动构建一次以生成首个 `main.min.css`）  
   - 结果：产出 `modules/core/presentation/assets/css/main.min.css`，静态资源请求恢复 200，登录页恢复完整样式。

4. [X] **验证登录链路**  
   - 操作：使用 `curl -i -c /tmp/iota-cookies -d 'Email=test@gmail.com&Password=TestPass123!' http://localhost:3200/login` 模拟表单登录；服务器返回 `302 /` 并下发 `sid` Cookie，日志显示 “User authenticated, creating session for user ID: 1”。  
   - 结论：认证路径和会话写入均正常，浏览器可成功跳转到主面板。

## 结果
- “内部服务器错误：<no value>” 消失，默认账号可顺利登录系统。
- `main.min.css` 已纳入构建流程，静态资源 404 不再出现。
- 未来如需重建环境，重复上述三个命令即可恢复数据库与前端依赖。
