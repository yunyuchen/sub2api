## Why

登录用户目前无法知道自己的用量在站内处于什么位置。仓库里两个最接近的功能都不承担这件事：管理端的 User Breakdown（用户用量明细）面向管理员，字段含邮箱与金额，按任意起止日期查询；渠道监控 V2 的用户 tab 是诊断视图，对非管理员一律匿名并把绝对用量清零（`ChannelMonitorV2Service.Users`），刻意不让人看出站点规模。两者都不是「所有人在同一时刻看到同一张榜」的社交视图。

同时，直接把用户名推上榜单是不可接受的：本仓库的 `users.username` 大多由 OAuth 静默回填，钉钉企业模式下就是真实姓名，Google 分支甚至可能落到邮箱全文。sub2api 由不同运营者部署，公开中转站与内部团队对「能否看到别人的名字和用量规模」的要求截然相反，单一策略两头都不讨好。因此本变更引入 Leaderboard（排行榜）：一个面向全体登录用户、按固定 Window（榜单窗口）聚合、字段受限的名次视图，由管理员在三档递增的 Leaderboard Mode（排行榜模式）中选择暴露程度，实名展示由用户自己在个人资料里选择开启。

## What Changes

- 新增用户侧页面与路由 `/leaderboard`：Window（今日 / 本周 / 本月）切换、Metric（排名指标）切换、My Rank（我的名次）卡片、至多 50 条 Leaderboard Entry（榜单条目）、Snapshot（榜单快照）更新时间与计算时区说明。侧边栏新增对应入口。
- 新增系统设置 Leaderboard Mode，取值 `off` / `anonymous` / `named`，**默认 `off`**：`off` 时普通用户访问 Leaderboard 接口返回 404、侧边栏不出现、前端路由守卫拦截；`anonymous` 时所有人以匿名形态出现，他人数值只显示相对第一名的整数百分比，Participant Count（参与人数）只给分档；`named` 时 Named Participation（昵称展示，默认开）为开且 username 合格的用户显示 username，所有数值与 Participant Count 精确。设置读不到时按 `off` 处理。
- 新增个人资料开关「在排行榜显示我的昵称」（列名 `leaderboard_named_participation`），**默认开启**，迁移 240 把既有用户一并回填为开。开启时校验 username（非空、非邮箱形态、长度与字符集、保留词拦截），不通过则拒绝开启；渲染时再校验一次，不合格回退匿名形态「第 N 位」。关闭的用户仍然参与排名，只是以匿名形态出现。
- Metric 只有两项：Total Tokens（总 tokens，input + output + cache_creation + cache_read）与 Successful Requests（成功请求数，`actual_cost > 0`），默认 Total Tokens。金额既不展示也不作为排序项。每行同时显示两个 Metric 的值，按选中的那个排序并高亮该列。
- Window 边界一律按站点时区计算，周从周一开始，页面标注时区名；没有自定义日期，也没有「全部时间」。
- Rank（名次）为竞争排名：等于该 Metric 值严格高于自己的用户数加一，并列同名次、其后跳号。匿名形态的展示名用 Ordinal（行序号，1..N 连续不跳号），用户 id 永不出现在展示名里。前三名徽章按 Rank 值判定。
- My Rank 在查看者不进前 50 时也展示，并配 Participant Count 作分母；查看者在该 Window 内没有任何用量时不给名次，只提示「暂无用量」；在前 50 内时该行高亮。
- 参与资格：管理员照常参与；`status = disabled` 与已软删除（`deleted_at` 非空）的用户被排除，也不计入 Participant Count。资格与展示名按响应时刻的 `users` 表当前状态判定，被禁用或删除的用户在下一次响应即从榜单条目消失，Participant Count 与 Rank 在下一轮重建后修正；不设清空 Snapshot 的钩子。
- 榜单数据来自后台每 5 分钟重建的 Snapshot，HTTP 请求路径只读、永不触发聚合。Snapshot 尚未生成时页面显示「正在计算」；超过 15 分钟未更新时显示陈旧警告而不是静默展示旧数据。
- Leaderboard Mode 为 `off` 时管理员仍可访问，响应带 Preview（预览）标记，页面显示「预览，普通用户不可见」横幅。
- 接口不下发 `user_id`、邮箱与任何金额字段；身份以结构化形式返回（`kind` 为 `self` / `anonymous` / `named`），文案由前端 i18n 渲染。
- 本变更不含 **BREAKING** 项：新增的 `users` 布尔列默认 `false`，新增的系统设置默认 `off`，公开设置负载与个人资料响应都只是新增字段，存量请求与存量行为不变。Successful Requests 与管理端 User Breakdown 的裸 `COUNT(*)` 是两个口径，但这是新增视图内部的口径选择，不改动 User Breakdown 现有行为。

## Capabilities

### New Capabilities

- `user-leaderboard`：Leaderboard 本体——Window 与 Metric 的取值与口径、Leaderboard Entry 的字段与上限、Rank 与 Ordinal 的定义、Display Name（展示名）的形态判定、My Rank 与 Participant Count、参与资格、空 / 加载 / 人数过少抑制 / 正在计算 / 陈旧五种非正常态，以及用户侧接口的请求与响应形状。
- `leaderboard-mode`：Leaderboard Mode 三档的语义与默认值、fail-closed 读取、写入白名单与读取侧归一化、`off` 下的 404 与管理员 Preview、Named Participation 开关（默认开、可关）与 username 校验规则。
- `leaderboard-snapshot`：Snapshot 的定义与重建契约——后台周期作业与选主、一条 SQL 同时产出三个窗口、Redis 派生结构与 key 命名及 TTL、原子切换、请求路径只读、陈旧上限，以及不设清空钩子的约束。

### Modified Capabilities

<!-- openspec/specs 目前为空，没有既有能力需要修改。 -->

## Impact

- **数据库**：`users` 新增布尔列 `leaderboard_named_participation`（`NOT NULL DEFAULT false`），新增迁移文件 `backend/migrations/238_user_leaderboard_named_participation.sql`（新增）；`backend/ent/schema/user.go` 增加对应 `field.Bool`，重新生成 ent 代码。不新增表，不改动 `usage_logs`。
- **后端**：设置接入按 `channel_monitor_mode` 的实际接入点逐一对齐——`service/domain_constants.go` 新增 `SettingKeyLeaderboardMode`、`service/setting_parse.go` 的默认值表与解析、`service/setting_update.go` 的写入归一化、`service/setting_public.go` 的读取键清单 / `GetPublicSettings` / `GetPublicSettingsForInjection` 与新增的 `normalizeLeaderboardMode`、`service/settings_view.go` 两处视图结构体、`handler/dto/settings.go` 两处、`handler/admin/setting_handler.go` 与 `handler/admin/setting_handler_update.go`，以及 `server/api_contract_test.go` 的 wantJSON 与 `handler/dto/public_settings_injection_schema_test.go`。新增 `service/leaderboard_service.go`（新增，查询与渲染）、`service/leaderboard_snapshot_service.go`（新增，后台作业）、`repository/leaderboard_cache.go`（新增，Redis 派生结构）、`repository/usage_log_repo_leaderboard.go`（新增，三窗口条件聚合，方法挂到 `service/account_usage_service.go` 里的 `UsageLogRepository` 接口）、`handler/leaderboard_handler.go`（新增）。`service/user_service.go` 的 `UpdateProfileRequest` 与 `UserUpdateFields`、`repository/user_repo.go` 的 `Update` 列投影、`handler/user_handler.go` 的 `UpdateProfileRequest` 与 `userProfileResponseFromService` 增加新字段。复用 `pkg/timezone`、`service/leader_lock.go` 的 `tryAcquireSingletonLeaderLock`、`repository/leader_lock_cache.go`、`service/timing_wheel_service.go` 的 `ScheduleRecurring`、`repository/usage_log_repo.go` 的 `usageLogSuccessFilterUL`。`service/wire.go` 新增 Provider 并在其中 `Start()`，`cmd/server/wire.go` 的 `provideCleanup` 新增该服务入参（仓库里只为副作用启动的后台服务被 wire 真正构造出来的唯一 sink），`cmd/server/wire_gen.go` 重新生成。
- **用户端 API**：新增 `GET /api/v1/leaderboard?window=&metric=`（`server/routes/user.go` 注册，走 `jwtAuth` + `BackendModeUserGuard` + `Global()` 限流 + 审计的标准链，再加 `panelRateLimiter.Heavy()` 与新增的 Leaderboard Mode guard）。`GET /api/v1/user/profile` 与 `PUT /api/v1/user` 的负载新增 `leaderboard_named_participation`。`GET /api/v1/settings/public` 新增 `leaderboard_mode`。
- **管理端 API**：`GET /admin/settings` 与 `PUT /admin/settings` 的负载新增 `leaderboard_mode`，写入侧白名单校验，非法值 400。不新增管理端路由——`off` 下的 Preview 走同一个用户端接口，由 guard 放行管理员。
- **前端**：新增 `src/views/user/LeaderboardView.vue`（新增）与 `src/api/leaderboard.ts`（新增）；`src/router/index.ts` 新增 `/leaderboard` 路由与 fail-closed 守卫；`src/components/layout/AppSidebar.vue` 新增导航项（`hideInSimpleMode` 与 `/usage` 一致）；`src/utils/featureFlags.ts` 新增枚举读取器 `getLeaderboardMode()` 与由它派生的布尔 flag；`src/types/index.ts` 的 `PublicSettings` 新增 `leaderboard_mode`；`src/api/admin/settings.ts` 两处与 `src/views/admin/SettingsView.vue` 新增设置控件；`src/components/user/profile/ProfileInfoCard.vue`、`src/components/user/profile/ProfileEditForm.vue` 与 `src/api/user.ts` 新增昵称展示开关；`src/components/user/leaderboard/displayName.ts` 是展示名的唯一出处。复用 `src/features/channel-monitor-v2/MonitorRankBadge.vue`。i18n 在 `src/i18n/locales/{zh,en}/` 的 `dashboard.ts`（页面文案新顶层块）、`common.ts`（`nav` 标签）、`admin/settings.ts`（设置项文案）同步补齐，并满足 `localeKeyCompleteness` / `localesNoKeyCollision` / `localesMessageCompile` 三个守门测试。
- **运行时（后台作业与 Redis）**：新增一个每 5 分钟运行的周期作业，经 `TimingWheelService.ScheduleRecurring` 注册、由 `tryAcquireSingletonLeaderLock` 保证多实例只跑一份；每轮一条 SQL 扫 `usage_logs`，扫描下界取 `min(月初, 周一)`。Redis 新增 6 个 ZSET（3 个 Window × 2 个 Metric）、3 个 Hash 与每个 Window 的更新时间，key 前缀沿用 `cfg.Dashboard.KeyPrefix` 做环境隔离，key 带窗口起点，TTL 取 `min(60 分钟, 距窗口结束)`。写入用 pipeline 写新 key 后 `RENAME` 原子切换。请求路径只读 Redis，不新增数据库查询压力。
