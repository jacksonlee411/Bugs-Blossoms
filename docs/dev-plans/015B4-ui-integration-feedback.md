# DEV-PLAN-015B4：UI 串联与反馈

**状态**: 已完成（2025-12-11 16:20，步骤 1-5 全部交付）  
**范围**: 草稿提交/状态反馈、错误处理、bot 联动与 a11y

## 目标
- 为所有草稿操作提供即时反馈与跳转入口，串联请求状态与 bot 结果。
- 统一错误处理，避免静默失败。
- 确保新增组件满足可访问性（a11y）要求。

## 实施步骤
1. [x] 反馈与跳转 —— 草稿操作（含 Unauthorized/PolicyInspector 提交）统一通过共享请求封装返回 toast/HTMX snippet，包含 `request_id` 与 SLA 倒计时（后端草稿/请求状态 API 提供 `estimated_sla_expires_at`，缺省用 i18n `sla.unknown`）；提供“查看草稿”跳转 `/core/authz/requests/{id}` 展示 diff/审批状态/bot 日志；HTMX 失败或非 HTMX 请求时使用重定向 + flash/服务器渲染 fallback；状态刷新用请求状态 API 轮询（15s，失焦暂停，回到前台立即触发一次），超过 2 分钟未更新则强制刷新一次。缓存范围仅限当前 tab 内存，命中缓存需满足“同 request_id 且距上次成功请求 <30s 且状态未达终态”，回到前台的首个请求绕过缓存；一旦进入终态或缓存超时即失效，复用现有 toast/flash 与节流，不另建通知系统。
2. [x] bot 联动 —— PolicyInspector/Unauthorized 显示 bot 状态，`status=failed` 时展示“重试 bot”链接且沿用统一反馈封装，不在组件内额外提示；重试需具备 `Authz.Requests.Update`（或等效）权限，附带单次 `retry_token`（60s 内有效），token 使用现有签名中间件生成的自包含 payload（含 `request_id`、`expires_at`、随机 nonce），后端按共享密钥校验并拒绝过期/重复，仍保持“同 request_id 每 60s 一次”限流；请求终态后隐藏重试入口，超限返回 `E_BOT_RATE_LIMIT`；未授权或 token 失效时返回统一 `showErrorToast`（含 i18n key），同时隐藏重试入口。
3. [x] 错误处理 —— 后端 4xx/5xx 返回 `HX-Trigger: {"showErrorToast": {"message": "<i18n key>", "code": "<错误码>"}}`，常用映射：`E_REQUEST_NOT_FOUND -> error.request_not_found`，`E_BOT_RATE_LIMIT -> error.bot_rate_limit`，`E_VALIDATION -> error.validation_failed`，`E_INTERNAL -> error.internal_retry`；`AUTHZ_INVALID_REQUEST`/base_revision 过期通过同通道附带最新 rev 并触发“刷新以更新权限基线”提示（HTMX/非 HTMX 都提供刷新 CTA），前端不做自动重试；非 HTMX 请求返回 JSON/HTML 标准错误页（含错误码与“查看详情”链接/flash），表单校验失败用 422 + 字段错误 partial；全局监听 `showErrorToast` 调用统一 toast，网络异常回退到默认“请求失败，请重试”；实现复用现有 serrors/统一响应封装，仅扩展 i18n key 与 HX-Trigger 输出。
4. [x] 可访问性 —— 本轮未新增模态/抽屉，新增按钮与轮询脚本已用键盘自检（Tab/Shift+Tab 聚焦、Enter 提交、Space 触发复制/重试），无新增 aria 要求；后续如新增弹窗按 `role="dialog"`/`aria-modal`/`aria-labelledby` 约定执行。
5. [x] 覆盖范围 —— PolicyInspector 草稿提交与 Unauthorized 补充请求均复用统一反馈、轮询、bot 重试；Core 控制器在非 HTMX 场景返回 JSON/标准错误页并携带错误码/i18n key/base_revision，flash fallback 保持可用，验收覆盖 HTMX + REST。

## 监控与日志
- 指标：`ui_toast_events_total{type=info|error,source=htmx|non_htmx,code}`；`bot_retry_total{result=ok|failed|rate_limited,code}`，label 仅允许固定枚举（code 使用小集合映射表，source/type/result 固定枚举），避免高基数；挂到现有 metrics 管线，无需新增系统。
- 日志字段：`request_id`、`subject`（脱敏/哈希）、`error_code`、`message_key`、`retry_token`、`next_retry_at`（如有）；不记录敏感原文，沿用当前日志格式/管道。

## 依赖
- 015A 的草稿/请求状态 API；015B1/015B2/015B3 的前端交互场景。

## 交付物
- 统一 toast 触发约定、跳转入口和 bot 状态联动逻辑。
- 错误处理与 i18n 映射示例，HTMX/非 HTMX fallback。
- bot 重试限流/幂等方案与可见性规则。
- a11y 辅助脚本/说明，错误处理示例。
- 监控与审计：toast/错误触发计数、bot 重试结果与原因日志。

## 验收
- 草稿操作（HTMX 与非 HTMX 入口）均有 toast/链接反馈，SLA 倒计时显示合理且随状态刷新。
- 错误处理返回用户可见文案与错误码，敏感信息未透出，失败日志可追溯。
- bot 失败场景可引导重试，限流/幂等生效，终态不再展示重试入口。
- 核心页面通过键盘可用性检查且无 axe 严重/致命级别违规，手动键盘清单通过。
- 相关日志/指标可查询（错误触发计数、bot 重试结果），便于排障。
