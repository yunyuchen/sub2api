## Context

动机见 proposal.md。与方案相关的现状：

- 用户可见排行的唯一先例是渠道监控 V2 的用户 tab。`ChannelMonitorV2Service.Users`（`backend/internal/service/channel_monitor_v2.go:662`）把 `Rank` 直接赋成行下标 `i + 1`，非管理员分支里把 `UserID` / `Email` / `Username` 全部清掉、展示名写成字面量 `"Me"` 与 `fmt.Sprintf("Other user #%d", i+1)`，并调用 `redactChannelMonitorV2Metric` 把绝对用量清零；`hideUserRankingForViewer` 由设置 `channel_monitor_hide_user_ranking` 控制，读取失败时 fail-open。Leaderboard 的名次规则、身份形态与失败方向都与它不同，不能直接复用这条链路。
- 管理端的按用户聚合是 `usageLogRepository.GetUserBreakdownStats`（`backend/internal/repository/usage_log_repo_trend.go:617`）：`LEFT JOIN users`、请求数是裸 `COUNT(*)`、`total_tokens` 为 `input + output + cache_creation + cache_read` 之和，同时返回 `email` 与三种金额，`ORDER BY` 走固定 allowlist。字段集与过滤口径都不适合直接对用户下发。
- 管理端仪表盘统计的缓存在 `DashboardService`（`backend/internal/service/dashboard_service.go`）：`cacheFreshTTL` / `cacheTTL` 双 TTL 的 stale-while-revalidate，后台刷新由结构体上的 `refreshing int32` 做 CAS 单飞——那是每个 service 实例一个整型，跨实例无效，且整条链挂在请求路径上。
- 站点时区工具在 `backend/internal/pkg/timezone`：`Location()`、`Name()`、`StartOfDay`、`StartOfWeek`（周一起算）、`StartOfMonth`；另有 `StartOfDayInUserLocation` 供按查看者时区计算的场景使用。
- 跨实例选主已有现成件：`backend/internal/repository/leader_lock_cache.go` 提供 Redis `SETNX` + 按 owner 比对删除的实现，`backend/internal/service/leader_lock.go` 的 `tryAcquireSingletonLeaderLock` 在缓存不可用时回落到 Postgres advisory lock。周期作业的样板是 `DashboardAggregationService.Start`：`TimingWheelService.ScheduleRecurring(name, interval, fn)` + 在 `fn` 内取锁。`service/wire.go` 的 `ProvideDashboardAggregationService` 里 `SetLeaderLock` 后直接 `Start()`。
- 成功请求的过滤约定是 `usageLogSuccessFilterUL = "ul.actual_cost > 0"`（`backend/internal/repository/usage_log_repo.go:30`），注释写明用于排除 tokens=0、cost=0 的失败占位记录；渠道监控 V2 的聚合 SQL 已经在用它。
- 带缓存的设置读取器样板是 `SettingService.IsBackendModeEnabled`（`backend/internal/service/setting_gateway_runtime.go:695`）：`atomic.Value` 存快照、60 秒 TTL、`singleflight` 收敛回源、读不到或出错时缓存一个 `false` 并缩短 TTL。相对地，`channelMonitorModeV2Guard`（`backend/internal/server/routes/admin.go:867`）是每请求裸查一次 settings 表。
- Redis key 的环境隔离前缀取自 `cfg.Dashboard.KeyPrefix`，用法见 `repository/dashboard_cache.go:20`（空值兜底 `sub2api:`，自动补冒号）。
- 前端 `frontend/src/utils/featureFlags.ts` 的注册表由 `defineFlag<K extends keyof PublicSettings>` 构成，`isFeatureFlagEnabled` 把读到的值 `as boolean | undefined` 后只认 `typeof raw === 'boolean'`，因此枚举设置注册进去会恒为 `false`；仓库已有的绕法是单独写读取器 `getChannelMonitorMode()`，再用 `isChannelMonitorV2Mode()` 这类布尔函数对外。侧边栏项通过 `makeSidebarFlag(flag)` 或直接传 `() => boolean` 的 `featureFlag` 字段接入（`components/layout/AppSidebar.vue`）。
- 公开路由的 fail-closed 守卫先例在 `frontend/src/router/index.ts:833`：`/model-plaza` 在守卫里先确保 `publicSettingsLoaded`，再只在「设置已成功加载且明确为 false」时拦截，瞬时加载失败交给后端兜底。
- 用户侧路由链在 `backend/internal/server/routes/user.go`：`authenticated` 组依次 `jwtAuth` → `middleware.BackendModeUserGuard(settingService)` → `panelRateLimiter.Global()` → `auditLog`；`/usage` 子组额外 `panelRateLimiter.Heavy()`，`channel-monitor-v2` 子组是 `Heavy()` + `channelMonitorModeV2Guard`。
- 身份文案的双语键已有先例：`channelMonitorV2.currentUser` 在 `frontend/src/i18n/locales/zh/channelMonitorV2.ts:24` 是「当前用户」、en 对应 `Current user`；同一页面上后端硬编码的 `"Me"` 与它混排过。
- 用户侧已有名次徽章组件 `frontend/src/features/channel-monitor-v2/MonitorRankBadge.vue`，接受 `rank` prop（`views/user/ChannelStatusV2View.vue:408` 在用）。
- `users` 表经 ent 的 `SoftDeleteMixin` 管理 `deleted_at`，`status` 列默认 `domain.StatusActive`；按列部分更新走 `service.UserUpdateFields` 的布尔开关（`backend/internal/service/user_service.go:98`）。

**重设计背景（2026-09-11，mockup 已确认）**：v1 的页面形态是「`AppLayout` 侧边栏壳 + 一张榜单表格」，与站点其余用户页同皮肤。用户看过 mockup 后选定了另一套视觉：把 `claude-relay-service` 的 Claude Editorial 皮肤（`web/admin-spa/src/styles/claude-tokens.css`、`views/InsightsClaudeView.vue`、`composables/useCountUp.js`、`composables/useAmbientMotion.js`）的视觉、动效与信息架构搬到 `/leaderboard`，页面从「一张榜」扩成「排行榜 & 洞察」：四张 Highlights（趣味卡）、用户排行、今日模型热度与近 30 天活跃度、用量趋势与今日时段分布与今日缓存命中。参考皮肤只借视觉与信息架构，不借其数据模型——它无鉴权、无脱敏、按金额排序，三条都与本变更的既有决策相反。本轮增量不推翻 D1–D14 的任何隐私与数据决定：金额仍然永不出现，`anonymous` 档仍然不下发任何他人或站点级绝对量，新增的 Highlights 与 Insights 一律按同一套档位规则裁剪（D18）。与这一轮相关的现状补充：

- 站点暗色由 `frontend/src/main.ts:25` 的 `initThemeClass()` 与 `frontend/src/components/layout/AppSidebar.vue:871` 的 `toggleTheme()` 共同维护，两处都是 `document.documentElement.classList.toggle('dark', ...)` 加 `localStorage['theme']`，因此页面级皮肤只能跟随 `html.dark`，不能另起一套主题状态。
- CSP 由 `backend/internal/server/middleware/security_headers.go` 统一下发，`requiredCSPDirectiveValues` 只放行了 Cloudflare、天御验证码、Stripe 与 Airwallex 四组域名，既没有 `fonts.googleapis.com` / `fonts.gstatic.com`，也没有任何第三方 `font-src`。
- 仪表盘预聚合表 `usage_dashboard_hourly` / `usage_dashboard_daily`（`backend/migrations/034_usage_dashboard_aggregation_tables.sql`）的桶边界其实是**站点时区**而不是表注释写的 UTC：`backend/internal/repository/dashboard_aggregation_repo.go:428` 用 `date_trunc('hour', created_at AT TIME ZONE $3) AT TIME ZONE $3` 落小时桶，同文件 `:500` 用 `(bucket_start AT TIME ZONE $5)::date` 落日期桶。两张表的 `total_requests` 都是裸 `COUNT(*)`，不带 `usageLogSuccessFilterUL`。
- 这两张表由 `DashboardAggregationService.Start()`（`backend/internal/service/dashboard_aggregation_service.go:92`）写入，受 `cfg.DashboardAgg.Enabled` 控制；关掉时整张表不再更新，也不会报错。

**v2 重设计背景（2026-09-12，mockup 已确认）**：v1 重设计（Editorial 皮肤，D15–D18）已实现并通过全部门禁。用户在设计画布上看过第二轮 mockup 后选定了「Geek」那一页：视觉从衬线编辑风换成终端极客风（`scratchpad/leaderboard-design-canvas/Main.dc.html` 为实名暗色、另有 `GeekAnonymousDark.dc.html` 与 `GeekNamedLight.dc.html`，配色 / 字号 / 间距 / 圆角等 CSS 常量与示例数据在同目录的 `gen_geek.py`），旧的 Editorial 画板留作对照、不再实现。同一轮里页面内容也扩了一圈：四张 Highlights 之下多一排六张用户维度的 Extremes（之最），榜单之前多一块只给本人看的 Viewer Stats（你的统计），站点级 Insights 从五块扩到十块（新增 `profiles`、`platforms_today`、`weekly_rhythm`、`composition_today`、`cache_trend_14`）。本轮不推翻 D1–D18 的任何隐私与数据决定：金额仍然永不出现，`anonymous` 档仍然不下发任何他人或站点级的绝对量，新增的每一个字段都按同一套档位规则逐条裁剪（D20、D22）。与这一轮相关的现状补充：

- `usage_dashboard_daily_users`（`backend/migrations/034_usage_dashboard_aggregation_tables.sql:59`）只有 `bucket_date` 与 `user_id` 两列，主键 `(bucket_date, user_id)`，另有 `bucket_date` 单列索引。「某人某天有没有用量」这个问题在这张表上是一次索引扫描，不必回 `usage_logs`；它与 `usage_dashboard_hourly` / `usage_dashboard_daily` 同受 `cfg.DashboardAgg.Enabled` 控制。
- `usage_logs` 上有 `image_count` 与 `video_count` 两列（列类型表见 `backend/internal/repository/usage_log_repo_insert.go:64` 与 `:70`），因此「带图或带视频的成功请求数」可以在既有的那条聚合里顺带数出来，不必另起一次扫描。
- 平台维度要 `JOIN accounts`：`usage_logs.account_id` → `accounts.platform`，渠道监控 V2 的聚合已经是这个写法（`backend/internal/repository/channel_monitor_v2_aggregation.go:185`）。区别是它用 `LEFT JOIN`，榜单这边与 D6 保持一致用 `INNER JOIN`（见 D22）。
- `backend/migrations` 下当前最大编号是本变更自己新增的 `238_user_leaderboard_named_participation.sql`，因此这一轮的新表取 `239`。
- v1 已落地、这一轮要复用或改写的件：`frontend/src/styles/leaderboard-tokens.css`、`frontend/src/composables/useCountUp.ts` 与 `useAmbientMotion.ts`、`frontend/src/components/user/leaderboard/` 下的十三个 `Lb*.vue`、`frontend/src/assets/fonts/leaderboard/` 下自托管的 Fraunces 与 Inter。换皮改的是这些文件的内容与增删，不另起一套目录。
**v3 重设计背景（2026-09-12，mockup 已确认）**：v2 重设计（Geek 皮肤，D19–D22）已实现并通过全部门禁。用户随后安装了 `design-taste-frontend-v1` skill（设计准则原文在 `.claude/skills/design-taste-frontend-v1/SKILL.md`，禁令清单：禁 Inter、衬线、紫色、霓虹、渐变字、纯黑、emoji 图标、三等宽卡片横排、居中 hero），并按它要求出第三版 mockup。用户在设计画布上确认的是「报表」那一版：视觉从终端极客风换成印刷报表风，页面从「顶栏 + 一串区块」重排成「masthead + 标题块 + 七个编号章节 + colophon 页脚」，每章左侧有一条承载口径说明的边注栏。三张已确认的画板是 `scratchpad/leaderboard-design-canvas/TasteNamedLight.dc.html`（实名亮）、`TasteNamedDark.dc.html`（实名暗）与 `TasteAnonymousDark.dc.html`（匿名暗），生成器 `scratchpad/leaderboard-design-canvas/v3/gen_taste.py` 的 `CSS` 常量与各 `build_chXX` 函数就是要移植的样式与结构；v2 的 Geek 画板与 v1 的 Editorial 画板一并留作对照、不再实现。**本轮只改前端**：接口、响应字段、档位裁剪规则、后端与迁移一行不动（响应形状同 v2，见 D20–D22），因此 D1–D14 的隐私与数据决定、D18 与 D20 的逐条裁剪规则全部照旧；换皮改的只是这些数据长什么样。与这一轮相关的现状补充：

- v2 已落地、这一轮要改写或删除的件：`frontend/src/styles/leaderboard-tokens.css`（`.gk` 皮肤）、`frontend/src/composables/useCrtEffect.ts`、`frontend/src/components/user/leaderboard/` 下的十八个 `Lb*.vue`、`frontend/src/assets/fonts/leaderboard/` 下自托管的 JetBrains Mono 与 IBM Plex Sans。换皮改的是这些文件的内容与增删，不另起一套目录。
- `frontend/src/i18n/locales/zh/dashboard.ts` 的 `leaderboard` 块里，`leaderboard.title` 已经是一个叶子字符串（`'用量排行榜'`），被 `frontend/src/router/index.ts:253` 的 `titleKey: 'leaderboard.title'` 当作路由标题读取。因此新增的标题块文案 MUST NOT 占用 `leaderboard.title.*` 这条路径——那会把叶子改成对象、连带打断路由标题（见 D23 的 i18n 命名）。
- vue-i18n 9.14.5 的路径解析支持 `'01'` 这种纯数字字符串作为对象键（实测 `t('leaderboard.chapters.01.note')` 可解析），因此章号可以直接做 i18n 键的一段。
- 三个 i18n 守门测试（`src/i18n/__tests__/localeKeyCompleteness.spec.ts`、`localesNoKeyCollision.spec.ts`、`localesMessageCompile.spec.ts`）只校验 zh / en schema 对齐、键不冲突与消息可编译，不校验「键有没有被用到」，因此按章号动态拼出来的键不会被它们误判为未使用；但也因此删键时必须自己把零引用的旧键清干净。


## Goals / Non-Goals

**Goals:**

- 榜单条目、Participant Count、My Rank 三处数字出自同一份 Snapshot，任何时刻互相自洽。
- 隐私控制在服务端强制，前端隐藏只是第二层；任何一环读不到设置都往更保守的方向倒。
- 聚合成本与在线用户数无关：请求路径只读 Redis，聚合频次固定为每 5 分钟一次、全站一份。
- 身份与参与资格反映当前状态，管理员禁用或删除一个用户后，下一次响应即生效，不必等下一次快照。
- 设置与 flag 的接入点与 `channel_monitor_mode` 逐一对齐，不新造一套注册机制。

**Non-Goals:**

- 不提供公开（免登录）访问，不做模型 / 分组 / API Key 维度的排行。
- 不提供自定义日期与「全部时间」窗口，不提供榜单长度选择器。
- 不展示任何金额，也不提供按金额排序的选项。（**已被 D24 取代**，2026-09-13：Cost（消费金额）成为第三个 Metric，金额与 tokens 走同一套档位规则。）
- 不在仪表盘加排行榜卡片（可作后续增量）。
- 不新增独立的「榜单展示名」字段，v1 用 `username` + 自选开关 + 校验。
- 不预先建按用户 × 天的预聚合表，除非上线前实测需要。

## Decisions

### D1：Leaderboard Mode 三档递增，读不到按 off

- 设置键 `leaderboard_mode`，取值 `off` / `anonymous` / `named`，默认 `off`。`off`：功能对普通用户既不可见也不可访问；`anonymous`：所有人匿名形态，他人数值只显示相对第一名的整数百分比，Participant Count 只给分档，本人行显示真实数值；`named`：开启 Named Participation 且 username 合格者实名，其余仍匿名，所有数值与 Participant Count 精确。
- 读取失败按 `off` 处理（fail-closed），与 `channel_monitor_hide_user_ranking` 的 fail-open 方向相反——Leaderboard 暴露的东西更多，名字一旦公开无法收回。
- 写入侧严格白名单，非法值 400；读取侧再归一化一次，非法值同样落到 `off`。
- 备选：布尔开关 + 直接复用 username。否决：见 ADR-0002——没有「开榜但不暴露规模」这一档，且会被动公开 OAuth 回填的真实姓名。
- 备选：完全照渠道监控先例，永远匿名并抹掉数值。否决：榜单退化成「我在哪」，失去社交意义。
- 备选：拆成「身份可见性」与「数值可见性」两个正交设置。否决：见 ADR-0002——管理员心智负担翻倍，三档递增已覆盖真实需求。
- `anonymous` 档下参与人数少于 5 时不展示榜单条目、只显示本人行。理由是小站点里「假名 + 相对百分比」也接近可辨认。这一条门槛值是设计阶段的假设，实现时可调。
- `anonymous` 档的 Participant Count 分档规则：取下界序列 5、10、20、50、100、200、500、1000、2000、5000、10000 中不超过实际人数的最大值，表达为「N+」（如 137 → 「100+」）；少于 5 时为「<5」，即抑制态。序列作为常量维护，实现时可调。

### D2：Named Participation 默认开启、用户可关，开启时校验 username

- `users` 新增布尔列，默认 `true`（迁移 240 把 238 建列时落成 `false` 的既有行一并回填为 `true`）；个人资料页加一行开关，文案「在排行榜显示我的昵称」。开启时校验 username，不通过则拒绝开启。校验规则（初始值，作为常量维护，实现时可调）：去首尾空白后 2–32 个字符；只允许 Unicode 字母、数字、`_`、`-`、`.` 以及词间的单个空格；不得含 `@`（邮箱形态）；不得命中保留词，保留词按大小写不敏感的子串匹配：`admin`、`root`、`system`、`official`、`support`、`sub2api`、「官方」、「管理员」、「客服」、「系统」。
- 渲染时再校验一次，不合格回退匿名形态——用户可能在开启后把 username 改成不合格的值。
- 不改动现有个人资料的 username 通用校验，避免波及注册与 OAuth 回填路径。
- 明确禁止「User #id」这类兜底（ADR-0002 的 Consequences 已把它列为明确禁止项）：用户 id 永不出现在展示名里，因为 id 是自增的、可用来推算注册规模。
- 关闭开关的用户仍然参与排名，只是以匿名形态出现；这不是「退出排行」开关。
- **默认值在 2026-09-12 由 `false` 翻成 `true`**（用户决策：「用户名字要写，不要写用户 #4 这样」）。ADR-0002 当初把它定为自选是因为 OAuth 会静默回填真实姓名；现在改为 opt-out——默认展示昵称、不接受的人自己去个人资料关掉。username 校验不变，邮箱形态仍被 `@` 规则挡在实名形态之外。见 ADR-0004。
- 备选：直接复用 username，不给退出通道。否决：见 ADR-0002 的 Considered Options——OAuth 会静默回填真实姓名甚至邮箱全文，退出通道必须保留（现在它是 opt-out 而不是 opt-in）。
- 备选：新增独立的「榜单展示名」字段并在首次进入时强制设置。否决：见 ADR-0002 的 Considered Options——v1 用 username + 开关 + 校验已够用，字段可日后再加。

### D3：Metric 只有两项，Successful Requests 用 actual_cost > 0

> **部分被 D24 取代**（2026-09-13）：Metric 从两项扩成三项，第三项是 Cost（消费金额，该 Window 的 `actual_cost` 之和）。本条里凡是「两项」「两个 Metric」的表述、「金额连排序项都不留」与「提供按金额排序但不显示金额是泄露路径」两句，以及末尾那条「Metric 固定为两项」的备选，都不再是实现依据；本条保留作决策沿革、不删。仍然成立的是三条口径：Total Tokens 含 cache 类 tokens、Successful Requests 用 `usageLogSuccessFilterUL`（`ul.actual_cost > 0`）、默认 Metric 是 Total Tokens。

- Total Tokens = `input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens`，与用户仪表盘、管理端 User Breakdown 的口径一致。
- Successful Requests 沿用 `usageLogSuccessFilterUL`（`ul.actual_cost > 0`）。
- 默认 Metric 是 Total Tokens。每行同时给出两个 Metric 的值，按选中的排序并高亮该列。
- 备选：排除 cache 类 tokens。否决：必须与仪表盘、User Breakdown 对得上，否则用户会拿两个页面对账。
- 备选：请求数用裸 `COUNT(*)`，与 `GetUserBreakdownStats` 完全一致。否决：`COUNT(*)` 会把失败占位行算进去，「刷失败请求」就成了冲榜路径。这是一处已知的口径差异，已写进 `CONTEXT.md` 的 Successful Requests 词条，页面上也要注明。
- 备选：提供按金额排序但不显示金额。否决：「按金额排但不显示金额」本身就是泄露路径，金额连排序项都不留。
- 备选：只按 tokens 排，不给第二个 Metric。否决：见决策日志 Q3/Q11，Metric 固定为 Total Tokens 与 Successful Requests 两项。

### D4：Window 按站点时区，周一起算

- 今日 / 本周 / 本月三个 Window，边界用 `timezone.StartOfDay` / `StartOfWeek` / `StartOfMonth` 计算，页面用 `timezone.Name()` 标注计算时区。
- 备选：按查看者浏览器时区解析，与管理端仪表盘的 `parseTimeRange` 一致。否决：见 ADR-0001——榜单的前提是所有人在同一时刻看到同一张榜，按查看者时区算会让两个人看到不同的「今日榜」，快照也要按时区分裂。
- 备选：让管理员单独配置一个「排行榜时区」。否决：站点时区已经存在，再加一个只会制造第二套口径。
- 备选：提供自定义日期范围。否决：固定窗口才能共享同一份快照。
- 备选：保留「全部时间」窗口配长缓存。否决：「全部」等于每次整表扫，且随着数据增长只会更慢。
- 已知限制两条，写进文档：DST 跳变日的窗口起点与站点既有日聚合行为保持一致，不做特殊处理；`usage_logs` 保留期短于本月窗口时本月榜会少算。

### D5：Rank 是竞争排名，Ordinal 只用于匿名展示名

- Rank = 该 Metric 值严格高于自己的用户数 + 1。并列同名次，其后跳号（1、1、3）。榜单条目与 My Rank 用同一条规则，因此两处永不互相矛盾。
- Ordinal 是榜内连续序号 1..N，唯一、不跳号，仅用于匿名形态的展示名「第 Ordinal 位」。切换 Metric 时同一人的 Ordinal 可能变化，这是它与 Rank 分开的直接后果。
- 前三名徽章复用 `MonitorRankBadge.vue`，按 Rank 值（≤3）判定而不是按行下标。
- 榜单条目最多 50 条，固定。
- 备选：匿名展示名直接用「第 名次 位」。否决：名次会并列，会出现两行「第 1 位」。
- 备选：按 `(window, user_id)` 哈希出稳定代号。否决：要处理碰撞，收益只是「跨窗口认得出同一个人」——而这恰好是匿名档不想要的。
- 备选：照渠道监控先例把 Rank 取成行下标 `i + 1`。否决：那样并列用户会被强行拉开，且与 My Rank 的算法对不上。
- 备选：榜单长度给 20 / 50 / 100 / 200 选择器。否决：选择器是管理端分析需求，不是社交视图需求。
- My Rank 在查看者不进前 50 时也展示，配 Participant Count 作分母；该窗口零用量时不给名次，只提示「暂无用量」。备选是把零用量用户排到末尾给一个巨大名次，否决：一群人并列同一个巨大名次没有信息量。

### D6：Snapshot 由后台作业重建，一条 SQL 出三个窗口

- 每 5 分钟一轮，经 `TimingWheelService.ScheduleRecurring` 注册；每轮先 `tryAcquireSingletonLeaderLock` 取锁，保证多实例只有一份在跑，Redis 不可用时回落到 Postgres advisory lock。
- SQL 形状：单次扫描 `usage_logs`，`WHERE created_at >= min(月初, 周一)`，`INNER JOIN users` 并过滤 `users.status <> 'disabled' AND users.deleted_at IS NULL`，`GROUP BY user_id`，用条件聚合一次产出六个数——每个 Window 各一组 `SUM(...) FILTER (WHERE ul.created_at >= $窗口起点)` 与 `COUNT(*) FILTER (WHERE ul.created_at >= $窗口起点 AND ul.actual_cost > 0)`。用 `INNER JOIN` 而不是 `GetUserBreakdownStats` 的 `LEFT JOIN`，是因为不合格用户与孤儿日志都不该进榜。
- Snapshot 只含 `user_id` 与数值，不含身份（数值在本轮重设计中从两个扩为四个，见 D17；D24 再加第十三段 Cost micros）。
- 备选：照仪表盘那套请求触发 + stale-while-revalidate + 双 TTL + 进程内 singleflight + Redis 刷新锁。否决：见 ADR-0003——要同时补四个机制才正确，且仪表盘的单飞是全 service 一个 `int32`、跨实例无效；冷启动和 key 过期时并发请求会各自跑一次整月扫描。
- 备选：请求路径实时查库。否决：面向全体登录用户，本月窗口约占 `usage_logs` 全表三分之一。
- 备选：每日重建一次。否决：今日榜会一整天不动。
- 备选：现在就建按用户 × 天的预聚合表。不是否决而是延后：上线前用生产量级数据跑一次 `EXPLAIN (ANALYZE, BUFFERS)` 量月窗口聚合并记录 p99，达到秒级就直接建表（或加覆盖索引 `created_at INCLUDE (...)`，`CREATE INDEX CONCURRENTLY` 的迁移用 `_notx.sql` 后缀，仓库已有 `062_add_scheduler_and_usage_composite_indexes_notx.sql` 等先例），不留给 v2。

### D7：Redis 存派生结构，key 带窗口起点

- 每个 Window × Metric 一个 ZSET（member = `user_id`，score = 该 Metric 的值），用于 Top 50（`ZREVRANGE`）、Participant Count（`ZCARD`）与竞争名次（`ZCOUNT (score +inf)` 严格大于后 +1）——Rank 的定义正好是 `ZCOUNT` 的语义。另有一个 Hash 存 `user_id` → 两个数值，供「每行显示两个 Metric」用；再存一个 Snapshot 更新时间。（本轮重设计把这个 Hash 扩为四个数值，并给每个 Window 增加一个 highlights key、给站点级洞察增加一个 insights key，见 D17；D24 把 Metric 扩成三个，因此每个 Window 是三个 ZSET，Hash 扩到十三段。）
- 写入用 pipeline 先写临时 key，再 `RENAME` 到正式 key 做原子切换，读者不会看到半份榜。
- key 命名带窗口起点：`leaderboard:v1:today:20260911`、`leaderboard:v1:week:20260907`、`leaderboard:v1:month:202609`，前缀沿用 `cfg.Dashboard.KeyPrefix` 做环境隔离。TTL 取 `min(60 分钟, 距窗口结束)`，跨零点 / 周一 / 月初时旧 key 自然作废；60 分钟的硬上限保证作业停摆时旧快照最多再服务一小时（15 分钟起已有陈旧警告），之后退回「正在计算」而不是无限期展示旧数据。
- 备选：整块 JSON 存一个 key。否决：每个请求都要反序列化整份，复杂度 O(N)；10 万用户量级下单份 5–10 MB。
- 备选：key 只按窗口名不带起点。否决：跨零点 / 周一 / 月初会串味，读到上一窗口的残留。

### D8：请求路径只读，身份与资格在响应时渲染

- HTTP 请求路径只读 Redis，永不触发聚合。Snapshot 缺失（key 不存在或 Redis 不可用）时返回「正在计算」这一明确状态，而不是空榜或 500。
- Snapshot 更新时间超过 15 分钟未推进时，响应仍给数据但带陈旧标记，页面显示警告，不静默展示旧数据。
- 数值与名次随快照冻结；展示名与参与资格在响应时按 `users` 表当前状态渲染——一次至多 51 行（Top 50 + 查看者）的按 id 批量查询。被禁用 / 删除的用户因此在下一次响应即从条目消失；Participant Count 与 Rank 的偏差在下一轮重建后修正。
- 不设「禁用 / 删除 / 切换模式时清空 Snapshot」的钩子。Snapshot 不含身份，切换模式不会泄露；下架已由渲染时剔除即时生效；而清空会让全站在下一轮重建前的最长 5 分钟里只看到「正在计算」，管理员批量禁用时尤甚。
- 备选：禁用 / 删除 / 切换模式时主动清空 Snapshot。否决：见上——收益只是 Participant Count 早几分钟修正，代价是全站断档。
- 备选：把展示名与资格一并冻结进快照。否决：「因投诉紧急下架某人」需要立刻生效，不能等下一次快照。

### D9：off 对普通用户返回 404，管理员可 Preview

- Leaderboard Mode 为 `off` 时，普通用户请求返回 404，不确认功能存在；前端路由守卫照 `/model-plaza` 的写法 fail-closed（先确保公开设置已加载，只在明确为 `off` 时拦截，瞬时加载失败交给后端 404 兜底）；侧边栏入口隐藏。
- 管理员在 `off` 下放行，响应带 `preview` 标记，页面显示「预览，普通用户不可见」横幅。
- Preview 下响应的 `mode` 回显 `off`、`preview` 为 `true`，条目按 `named` 档的身份形态与数值精度渲染，`participant_count` 为精确整数。理由沿用 Q15/Q32 对 Preview 的定位：管理员要预览的正是开启后最开放的形态，若只按 `anonymous` 渲染，他仍要靠「先开后关」才能看到 `named` 的效果，而那恰是 Preview 要消除的短暂真实泄露。`off` 档下侧边栏入口对所有角色隐藏，管理员经直接访问 `/leaderboard` 进入 Preview。
- 模式开启后管理员与普通用户看到完全相同的数据，Leaderboard 不做角色分支——需要全字段（邮箱、三种金额口径、任意日期）的管理员走 User Breakdown。
- 备选：只在前端隐藏。否决：隐私控制必须服务端强制。
- 备选：返回 403。否决：403 会确认功能存在。
- 备选：`off` 下一律拒绝、不给管理员预览。否决：会逼管理员「先开后关」来看效果，造成一段真实泄露。
- 备选：管理员看到带邮箱与金额的全字段视图。否决：全字段在 User Breakdown 已有，两套口径只会混淆。

### D10：身份以结构化形式下发，文案由前端 i18n 渲染

- 每个条目的身份是 `identity{kind: self|anonymous|named, username?}`，后端不拼展示名字符串。`self` 由前端渲染成查看者本人的 `username`（缺席或全空白时才回退「当前用户」），`anonymous` 渲染成「第 Ordinal 位」，`named` 直接用 `username`；这四条分支只在 `frontend/src/components/user/leaderboard/displayName.ts` 里实现一次。（D25 起身份多一个可选的 `avatar_url`，受与 `username` 完全相同的规则约束：只有 `named` 形态可能带，`self` 的头像同样由前端从查看者自己的资料里取，后端不下发。）
- 术语沿用 `channelMonitorV2.currentUser`（「当前用户」/「Current user」），不再出现字面量 `"Me"`。
- 响应不下发 `user_id` 与邮箱；金额只有 Cost 这一个口径，按与 tokens 相同的档位规则下发（见 D24），其它金额口径（`total_cost` 之类）一律不出现。身份字段从 D25 起是 `{kind, username?, avatar_url?}`：`avatar_url` 只在 `named` 形态下可能出现，`self` 与 `anonymous` 一律不下发，其余不变。
- 备选：后端直接拼 `display_name` 字符串。否决：与 zh / en 双语互斥；渠道监控页面已经因此出现过 `"Me"` 与「当前用户」混排。

### D11：设置接入按 channel_monitor_mode 逐点对齐，前端用枚举读取器派生布尔 flag

- 后端接入点：`service/domain_constants.go` 的 `SettingKey*` 常量、`service/settings_view.go` 的两处视图结构体、`service/setting_parse.go` 的默认值表与解析、`service/setting_update.go` 的写入归一化、`service/setting_public.go` 的读取键清单与 `GetPublicSettings` / `GetPublicSettingsForInjection`、`handler/dto/settings.go`、`handler/admin/setting_handler.go` 与 `handler/admin/setting_handler_update.go`（后者的 `*string` 指针语义保证「未提交该字段」不会把值刷掉），以及 `server/api_contract_test.go` 的 wantJSON 与 `handler/dto/public_settings_injection_schema_test.go`。归一化函数照 `normalizeChannelMonitorMode` 的形状写，区别只在默认值方向。
- 前端：`featureFlags.ts` 的 `FeatureFlags` 注册表不能直接登记枚举——`isFeatureFlagEnabled` 只认 `typeof raw === 'boolean'`，枚举值会恒为 `false`。因此新增枚举读取器 `getLeaderboardMode()`，再由它派生一个「`mode !== 'off'`」的布尔函数交给侧边栏，正如 `getChannelMonitorMode()` 与 `isChannelMonitorV2Mode()` 的关系。
- 备选：照 `featureFlags.ts` 文件头「Adding a new flag」的 11 处清单接入。否决：那份清单已过期，且只适用于布尔开关。
- 备选：直接 `defineFlag` 一个枚举键。否决：注册表会把值 cast 成 boolean，结果恒 `false`。

### D12：mode guard 用带短 TTL 的进程内缓存读取器

- guard 读取 Leaderboard Mode 时走一个照 `IsBackendModeEnabled` 形状写的读取器：`atomic.Value` 存快照 + 短 TTL + `singleflight` 收敛回源，读不到或出错时缓存 `off` 并缩短 TTL。代价是管理员切换模式后有秒级延迟，可接受。
- 切换模式后那几秒里的旧档渲染无法靠清空 Snapshot 消除——身份与精度都在响应组装层按档位决定，Snapshot 本身不含身份——因此不设清空钩子，接受秒级延迟；读取器 TTL 取 5 秒量级。
- 备选：照 `channelMonitorModeV2Guard` 每请求裸查一次 settings 表。否决：那条先例本身就是一处已知的每请求 DB 查询，Leaderboard 面向全体登录用户，不该把它放大。

### D13：独立路由与侧边栏入口

- 新路由 `/leaderboard`（用户侧，`requiresAuth: true`，带 `meta.titleKey`），侧边栏新增一项，随 D11 的派生布尔 flag 显隐，`hideInSimpleMode` 与 `/usage` 保持一致。
- 不在仪表盘加卡片。
- 页面形态在本轮重设计中改为全屏独立页（见 D15）；路由、守卫、侧边栏入口与 `hideInSimpleMode` 的决定不变。**（2026-09-14 已改回套 `AppLayout` 的应用内页，见 D15 标题下的修订块；本条其余部分照旧。）**
- 备选：做成 `/usage` 页面里的一个 tab。否决：窗口切换、Metric 切换、My Rank 卡片装不进去，且 `/usage` 是「我的记录」语义，榜单是「所有人」语义。
- 备选：只在仪表盘放一张卡片。否决：同上；卡片可作后续增量。

### D14：单一 GET 接口，查询参数表达 Window 与 Metric

- `GET /api/v1/leaderboard?window=today|week|month&metric=total_tokens|successful_requests|cost`，两个参数都有默认值（`today`、`total_tokens`），非法值 400。`cost` 这一档是 D24 新增的。
- 中间件：用户侧标准链（`jwtAuth` + `BackendModeUserGuard` + `Global()` 限流 + 审计）+ `panelRateLimiter.Heavy()`（与 `/usage/*`、`channel-monitor-v2` 一致）+ Leaderboard Mode guard。
- 响应字段：`window`、`metric`、`mode`、`preview`、`timezone`、`status`（`ready` / `computing`）、`stale`、`snapshot_updated_at`、`participant_count`（`named` 档与 Preview 下为精确整数，`anonymous` 档下为分档字符串）、`entries[]{rank, ordinal, identity, total_tokens | total_tokens_relative_percent, successful_requests | successful_requests_relative_percent, cost | cost_relative_percent, is_self}`、`entries_suppressed`、`my_rank{rank, total_tokens, successful_requests, cost} | null`（`cost` 两处均由 D24 新增，单位 USD）。
- `status` / `stale` / `entries_suppressed` 三个字段让前端能判定「正在计算」、陈旧与「人数过少暂不展示」三种状态，而不必从「`entries` 是空数组」反推——抑制态与空态的 `entries` 都是空数组，没有显式信号就无法分开渲染两套文案。
- `anonymous` 档下他人条目里的绝对数值字段缺席，只给 `total_tokens_relative_percent`、`successful_requests_relative_percent` 与 `cost_relative_percent`（各自相对该 Metric 第一名的整数百分比，第一名为 `100`）；本人条目与 `my_rank` 始终是真实数值。这样「档位决定字段是否存在」而不是「档位决定字段被清零」，客户端无法把缺席误读成 0。

### D15：`/leaderboard` 是套 `AppLayout` 的应用内页（2026-09-14 修订）；侧边栏入口保留

> **2026-09-14 修订**（用户第二条指令「把这个做成不用跳转的内页，省得用户再跳转出去，再跳转回来」）。本条原标题是「`/leaderboard` 是全屏独立页，不再套 `AppLayout`」，下面前四条 bullet 描述的「全屏独立页、不套 `AppLayout`、页面自带站点顶栏与主题开关、『返回仪表盘』主按钮」**MUST NOT 再作为实现依据**，保留作决策沿革，不删。现状写在本条末尾那条修订 bullet 里：页面套 `AppLayout`，侧边栏与站点顶栏由外壳提供，页内没有任何页面私有的站点导航与「返回仪表盘」入口，`.rp` 是外壳内容区里的一块内容面板（明暗随站点，见 D23 的再修订注记）。仍然成立并原样承接的是三条：路由与守卫不变、侧边栏入口保留并随 Leaderboard Mode 显隐、`hideInSimpleMode` 与 `/usage` 一致。

- 页面自带顶栏：站名 + 斜体 tagline「团队用量观察」+ 当前页标记「排行榜」+ 环境动效开关 + 主题开关 + 「返回仪表盘」主按钮。`frontend/src/views/user/LeaderboardView.vue` 去掉 `<AppLayout>` 外壳（现为该文件第 2 行与第 166 行），自己从页面根节点起渲染整页。
- 这条取代 D13 里「页面与 `/usage` 共用同一套 `AppLayout` 外壳」的隐含表述。D13 的其余部分不变：路由仍是 `/leaderboard`（`requiresAuth: true`、带 `meta.titleKey`），守卫仍然 fail-closed，侧边栏「用量排行」入口保留并随 `leaderboard_mode !== 'off'` 的派生布尔 flag 显隐，`hideInSimpleMode` 仍与 `/usage` 一致。变的只是点进来之后看到的是一张独立页，而不是内容区换皮。
- 顶栏的主题开关复用站点既有状态：`document.documentElement.classList.toggle('dark', ...)` + `localStorage['theme']`，与 `AppSidebar.vue:871` 的 `toggleTheme()` 语义一致，因此在本页切换主题后回到其它页面不会出现两套主题。
- 备选：留在 `AppLayout` 里只换内容区皮肤。否决：两套视觉体系并排会互相打架（侧边栏是站点的 Tailwind 皮肤，内容区是 Editorial 衬线皮肤），用户看过 mockup 后明确选了独立风格。
- 备选：做成独立页并顺手把侧边栏入口去掉。否决：入口一旦消失，功能就只能靠直链进入；D13 的入口决策与档位显隐逻辑本身没有问题，不该被页面形态牵连。
- **2026-09-14 修订（用户第二条指令「把这个做成不用跳转的内页」）：本条的「全屏独立页、不套 `AppLayout`、页面自带站点顶栏与『返回仪表盘』入口」MUST NOT 再作为实现依据。** 页面改回套 `AppLayout` 的应用内页：侧边栏与站点顶栏由外壳提供，页面私有的站点导航组件（`LbSiteNav.vue`）删除，页内不再有任何「返回仪表盘」入口，`.rp` 从页面根降级成外壳内容区里的一块深色内容面板（无 `min-height:100vh`，底纹伪元素由 `fixed` 改为 `absolute`，否则会连侧边栏一起盖住）。被否决的那条「两套视觉体系并排会打架」不再成立：新皮肤是站点自己的 spool 品牌皮肤，与外壳同源。本条其余部分（路由、守卫、侧边栏入口、`hideInSimpleMode`）照旧；深色专属见第 16 节与 D16 的修订。**同日再修订**：「深色专属」已被用户推翻（「没有做主题适配」），面板的明暗改回跟随站点的 `html.dark`，由样式层推导，见 D23 的再修订注记与 `tasks.md` 第 18 节。

### D16：视觉 token 作用域在页面根节点，暗色跟随 `html.dark`，字体自托管

> **已被 D19 取代**（v2 重设计，2026-09-12）。Editorial 皮肤的具体形态——`.cr-theme` 作用域、Fraunces + Inter、🥇🥈🥉 奖牌、`.cr-ambient` 环境动效——不再是实现依据，本条保留作决策沿革，不删。仍然成立并由 D19 原样承接的是四条约束：视觉 token 定义在页面根节点这一层而不是 `:root`、暗色跟随站点的 `html.dark` 而不是 `prefers-color-scheme`、字体因 CSP 未放行 Google Fonts 而必须自托管、前三名徽章按 `rank` 值判定而不是行下标或 Ordinal。

- 新增 `frontend/src/styles/leaderboard-tokens.css`，所有变量定义在页面根节点这一层（`.cr-theme`）而不是 `:root`，由 `LeaderboardView.vue` 自己 `import`，不进全局样式入口，因此其它页面不受影响，也不会与站点既有的 Tailwind 变量抢名字。
- 暗色跟随站点现有的 `html.dark`：选择器写成 `.dark .cr-theme` 与 `.cr-theme.dark`。不用 `prefers-color-scheme` 单独判定——那会与顶栏和侧边栏的主题开关不同步，出现「站点是亮色、本页是暗色」。
- 字体：标题、名次数字与大数字用 Fraunces，正文用 Inter，不用 JetBrains Mono。站点 CSP 没有放行 `fonts.googleapis.com` / `fonts.gstatic.com`，`@import` 会被静默拦截（页面不报错，字体直接不生效），因此**自托管** woff2 到 `frontend/src/assets/fonts/`（两款都是 OFL 许可），`@font-face` 只在 `leaderboard-tokens.css` 里声明，配 `font-display: swap`，并给出 `'Source Serif 4', Georgia, serif` 与 `system-ui, -apple-system, sans-serif` 两条兜底栈；字体文件拿不到时页面仍然可读。
- 备选：往 CSP 里加 `fonts.googleapis.com` / `fonts.gstatic.com`。否决：为一张页面放宽全站 `style-src` 与 `font-src`，并给所有部署引入一个必须可达的外部依赖（内网与国内网络都可能拿不到）。
- 备选：只用系统字体，不引 Fraunces。否决：整套视觉的识别度就在衬线标题与衬线名次数字上，退回系统字体等于没搬皮肤。
- 动效清单（照参考实现，落在 `leaderboard-tokens.css` 与两个 composable 里）：条形从 0 生长（`.cr-bar > div` 读 `--cr-bar-w`，根节点加 `.cr-grown` 后由双 rAF 触发过渡）、榜单行与四张卡的 stagger 进场（`.cr-stagger > *` 按 `--i` 递延）、数字滚动累加（`frontend/src/composables/useCountUp.ts`：rAF + easeOutExpo、600ms）、滚动进场 reveal（`.cr-reveal` + `IntersectionObserver`）、骨架屏 shimmer。
- 可开关的环境动效 `.cr-ambient`（`frontend/src/composables/useAmbientMotion.ts`）：aurora 背景、卡片鼠标光晕（`pointermove` 写 `--mx` / `--my`）、进度条流光、金牌呼吸。模块级状态跨组件共享，`localStorage` 持久化，默认值取 `!prefers-reduced-motion`，顶栏给一个开关。
- `prefers-reduced-motion: reduce` 时全局关闭：样式层用媒体查询把 `.cr-theme` 下所有 `animation-duration` / `transition-duration` 压到 `0.01ms`，脚本层 `useCountUp` 直接跳终值、`useAmbientMotion` 默认关闭。
- 奖牌用 🥇🥈🥉 emoji（参考皮肤的既定元素），第 4 名起用衬线零填充数字（`01`–`50`）。前三名判定仍按 Rank 值 ≤ 3，与 `user-leaderboard` 的「前三名徽章按 Rank 值判定」完全一致，MUST NOT 按行下标或 Ordinal。代价是不再复用 `frontend/src/features/channel-monitor-v2/MonitorRankBadge.vue`——那是 Tailwind 皮肤的彩色徽章，与 Editorial 皮肤并排会破相；被替换的只是渲染形态，判定规则原样保留。
- 备选：继续复用 `MonitorRankBadge.vue`，只改配色。否决：mockup 的奖牌是 emoji + 衬线序号两种形态并存，徽章组件表达不了第二种，改到能表达等于重写。

### D17：Highlights 与 Insights 在同一轮作业里算好，请求路径仍只读 Redis

- 每用户 Hash 从两个数扩为四个：`total_tokens`、`successful_requests`、`input_tokens`、`cache_read_tokens`。同一条聚合 SQL 每个窗口多两个 `SUM(...) FILTER (...)`，`usagestats.LeaderboardAggregateRow` 相应多六个字段，`service.LeaderboardUserMetrics` 多 `InputTokens` 与 `CacheReadTokens` 两个字段。Hash value 的编码从 `"tokens,requests"` 扩成 `"tokens,requests,input,cache_read"`，仍是逗号分隔。ZSET 仍然只有两个 Metric（D24 之后是三个，见该条）——新增的两个数只用来算 Cache Hit Rate（缓存命中率），不参与排名。
- Cache Hit Rate = `cache_read_tokens / (input_tokens + cache_read_tokens)`，按 Window 聚合。某用户该窗口 `input_tokens + cache_read_tokens` 为 0 时没有命中率（字段缺席），MUST NOT 记成 0。缓存的收益一律用命中率与命中 tokens 表达，不折算成金额——「省下多少钱」是估算，与 D24 下发的 Cost（实际计费金额）不是一回事，两者混在一起会让页面上出现两种口径的金额。
- 每个 Window 一份 highlights，重建时算好，存成一个 JSON 串：key 是 `<cfg.Dashboard.KeyPrefix>leaderboard:v1:<window>:<窗口起点>:highlights`，与该 Window 的 ZSET / Hash 同一轮 pipeline 写入、同样先写临时 key 再 `RENAME`、同一个 TTL。内容是 `top_tokens`、`top_requests`、`cache_king`、`site` 四块，只存 `user_id` 与数值，不存身份——身份仍在响应时按 `users` 当前状态渲染，与 D8 一致。
- `cache_king`（效率之星）要遍历该窗口全部用户才能选出，因此只在重建时做一次；门槛是该窗口 Successful Requests 大于 5，达不到门槛的用户不参与评选，全窗口无人达标时整块为 null。`dominant_model`（该用户该窗口请求最多的模型）需要一条按 `(user_id, model)` 的聚合，只对 `cache_king` 这一个用户查一次。
- 站点级 insights 与 Window 无关，一个 key：`<cfg.Dashboard.KeyPrefix>leaderboard:v1:insights:<今日窗口起点>`，JSON 串，TTL 同样取 `min(60 分钟, 距今日窗口结束)`。key 带今日起点是为了跨零点自然作废，理由与 D7 相同。五个区块的来源：
  - `models_today`：今日 Top 8 模型，直接扫今日 `usage_logs`，`GROUP BY model`，计数带 `usageLogSuccessFilterUL`，因此是**成功落账**口径，字段名就叫 `successful_requests`；
  - `daily_30`：近 30 天每天的请求数与 total tokens，来自 `usage_dashboard_daily`；
  - `hourly_today`：今日 24 个小时桶，来自 `usage_dashboard_hourly`；
  - `cache_today`：今日全站命中率与 `cache_read_tokens` / `input_tokens`，由 `hourly_today` 的同名列汇总，不另查一次；
  - `month`：本月累计 total tokens 与「较上月」的百分比，由 `usage_dashboard_daily` 按两段日期区间各求一次和算出；上月的绝对量只用于算百分比，MUST NOT 下发。
- 两张预聚合表的桶边界本来就是站点时区，不需要把 UTC 桶再映射一次，也不需要为跨 UTC 日多读一天——表注释里的「UTC buckets」是过时的，实现以 `dashboard_aggregation_repo.go:428` 与 `:500` 的 SQL 为准。
- 口径差异写明：预聚合表的 `total_requests` 是裸 `COUNT(*)`，不带 `usageLogSuccessFilterUL`，因此 `daily_30` 与 `hourly_today` 的计数是**全部请求**，字段名就叫 `requests`，与榜单和 `models_today` 的 `successful_requests` 不是同一个口径。页面在这两个区块下注明，footer 的口径说明也带上这一句。备选是给这两块也做成功落账口径。否决：那要绕开预聚合表回去扫 `usage_logs`，请求路径只读 Redis 的前提虽然还在，但后台每轮多两次范围扫描，代价与收益不成比例。
- 预聚合缺行（`cfg.DashboardAgg.Enabled` 关着，或保留期没覆盖到那段日期）时，对应区块整块返回 null，前端隐藏该区块；MUST NOT 用 0 填充，也 MUST NOT 让整个响应失败。`models_today` 不依赖预聚合表，因此不受这条影响。
- 两张预聚合表**读取出错**与缺行同等对待：重建时把对应区块降级为 null 并记一条日志，本轮照常写入三个窗口的快照。它们是仪表盘的派生物，读不动不说明榜单数据有问题，没有理由让整轮重建被连坐——页面上少一块洞察，好过全站在下一轮之前只能看一份越来越旧的榜。硬失败因此只剩两条：`AggregateLeaderboardWindows`（榜单本体）与 `LeaderboardTopModelsToday`（与榜单同源、直接扫 `usage_logs`），它们出错时整轮不切换，正式 key 保持上一轮的完整内容。
- 响应扩展两个顶层字段：`highlights{top_tokens, top_requests, cache_king, site}` 与 `insights{models_today[], daily_30[], hourly_today[], cache_today, month}`；`status` 为 `computing` 时 `highlights` 一并为 null。`identity` 仍是结构化的 `{kind, username?}`（D25 起读作 `{kind, username?, avatar_url?}`，多出来的 `avatar_url` 只在 `named` 形态下可能出现，Highlights 的身份从 `slots` 继承，不另查一次），绝对量字段仍用指针 + `omitempty` 表达「字段是否存在」，与 D14 的 `entries` 同一套写法。
- 备选：highlights 与 insights 在请求路径上现算。否决：`cache_king` 要遍历全窗口用户、`models_today` 要扫今日 `usage_logs`，两条都是 D8「请求路径只读」明令禁止的。
- 备选：把 insights 也按 Window 分三份。否决：模型热度、时段分布与缓存命中都是「今日」语义，近 30 天与本月累计本来就跨窗口；分三份只是把同一份数据存三遍。

### D18：Highlights 与 Insights 的数值裁剪逐条对齐 D1 的三档

- `named` 档与 Preview：Highlights、模型热度、趋势、时段、缓存全部可给绝对值；`site` 给 Total Tokens、Successful Requests、Cost（D24 新增）与精确的 Participant Count。
- `anonymous` 档：任何**他人的**或**站点级的**绝对量都不下发，一个都不留。逐块如下：
  - Highlights 的领先者身份用榜单 Ordinal 假名：`identity.kind` 为 `anonymous`、`ordinal` 为其在当前 Window + Metric 榜单上的 Ordinal，前端渲染成「第 Ordinal 位」；该用户不在前 50 时 `ordinal` 为 null，前端渲染成「榜外用户」。MUST NOT 用 Rank 代替 Ordinal（Rank 会并列，会出现两张卡都写「第 1 位」），更 MUST NOT 出现用户 id。
  - Highlights 的数值只给 `share_percent`（占全站该 Metric 的百分比，0–100 整数）与 `lead_percent`（比第 2 名多出的百分比，整数）；`total_tokens` / `successful_requests` 缺席。这两个相对量在 `named` 档同样下发——mockup 的实名卡也用到了 `share_percent`。
  - `cache_king` 的 `cache_hit_rate` 与 `dominant_model` 两档都给：命中率本身是比率，模型名不是某个人的绝对用量。
  - `site` 只给分档后的 `participant_count`（规则与 D1 的下界序列一致）、`cache_hit_rate` 与 `peak_hour`；`total_tokens`、`successful_requests` 与 `cost` 缺席。
  - `models_today` 只给 `share_percent`，`successful_requests` 缺席。
  - `daily_30` 只给 `relative_percent`（相对这 30 天里最高一天的 0–100 整数百分比），`requests` 与 `total_tokens` 缺席。`relative_percent` 两档都给——热力图的色阶与趋势条的宽度都靠它，`named` 档同样要用。
  - `hourly_today` 只给 `hour` 与 `relative_percent`（相对峰值小时），`requests` 缺席；峰值小时由 `relative_percent` 为 100 的桶给出，两档都可见。
  - `cache_today` 只给 `cache_hit_rate`，`cache_read_tokens` 与 `input_tokens` 缺席。
  - `month` 只给 `change_percent`（较上月的整数百分比，可为负），`total_tokens` 缺席；页面上「本月累计 165M」相应换成「本月累计 · 较上月 +18%」。
- 「你的位置」的提示语受同一条约束：`anonymous` 档下 MUST NOT 含任何他人的绝对量，只能用相对第一名的百分比表达（如「相对第一名 2%，第 10 名是 3%」）；`named` 档可以用绝对差额（如「再多 24K tokens 就能进前 10」，Cost 下是「再消费 $X 进前 10」，见 D24）。文案由前端按档位选，后端只下发数值。
- 本人条目、`my_rank` 与「你的位置」的两个数值在任何档位都是真实值——这条是 D14 与 `leaderboard-mode` 已有的规则，Highlights 与 Insights 不改变它。查看者本人恰好是某张 Highlights 卡的领先者时，该卡的 `identity.kind` 为 `self`，但数值仍按当前档位裁剪：这张卡是给所有人看的同一份数据，不是「我的数据」，若在这里放行绝对值，等于给全站开了一个「只要自己第一就能看到自己绝对量被公开展示」的口子。
- 备选：`anonymous` 档干脆不给 Highlights 与 Insights。否决：整页只剩一张榜，重设计的信息架构塌掉一半；相对量与比率本来就不泄露绝对规模。
- 备选：把绝对量按档位清零而不是缺席。否决：与 D14 同一个理由——客户端会把 0 误读成「真的是 0」。
- 备选：`share_percent` 给一位小数（mockup 里是 19.6%、47.6% 这样的示例值）。否决：D18 里所有百分比统一成整数，只有 `cache_hit_rate` 是 0–1 的比率并按一位小数渲染；两种精度并存会让「这个百分比能不能反推绝对量」变成逐字段判断题。mockup 的小数是示例渲染，不是口径。

### D19：视觉体系换成极客风，取代 D16 的 Editorial 皮肤

> **已被 D23 取代**（v3 重设计，2026-09-12）。Geek 皮肤的具体形态——`.gk` / `--gk-*` 作用域、JetBrains Mono + IBM Plex Sans、24px 网格背景、状态栏 chip、`$ leaderboard --window …` 命令行标题与各区块的命令行小标题、`[1]` / `#04` 徽标、LED 分段条、CRT 扫描线与 `.gk-reveal` 滚动进场——不再是实现依据，本条保留作决策沿革，不删。仍然成立并由 D23 原样承接的是四条约束：视觉 token 定义在页面根节点这一层而不是 `:root`、明暗跟随站点的 `html.dark` 而不是 `prefers-color-scheme`、字体因 CSP 未放行 Google Fonts 而必须自托管、前三名的判定按条目的 `rank` 值而不是行下标或 `ordinal`。

- 页面根节点的 class 从 `.cr-theme` 改为 `.gk`，变量前缀从 `--cr-*` 改为 `--gk-*`，`frontend/src/styles/leaderboard-tokens.css` 整份改写。作用域规则不变：变量仍定义在页面根节点这一层而不是 `:root`，仍只由 `frontend/src/views/user/LeaderboardView.vue` 自己 `import`，不进全局样式入口。配色、字号、间距、圆角、LED 条、徽标、热力图阶梯、状态栏、命令行标题、`#` 注释与 `//` 注释的取值一律照 `gen_geek.py` 里的 CSS 常量。
- 明暗的判定方向反过来：Geek 皮肤以暗色为底，`.gk` 上定义的就是暗色变量，站点处于亮色时页面根节点追加 `.light`，由 `.gk.light` 覆写同名变量。这个 class 由 `LeaderboardView.vue` 按 `document.documentElement.classList.contains('dark')` 计算，并在本页的主题开关切换时同步更新。D16 的约束本身不变：不另起一套页面私有的主题状态，也不用 `prefers-color-scheme` 单独判定，否则会出现「站点亮色、本页暗色」。
- 字体换成 JetBrains Mono（标题、数字、标签、表格、命令行）与 IBM Plex Sans（正文），两款都是 OFL。CSP 的结论与 D16 一致（`security_headers.go` 没放行 `fonts.googleapis.com` / `fonts.gstatic.com`，`@import` 会被静默拦截），因此同样自托管 woff2 到 `frontend/src/assets/fonts/leaderboard/`，`@font-face` 只写在 `leaderboard-tokens.css` 里、配 `font-display: swap`，兜底栈分别是 `ui-monospace, SFMono-Regular, Menlo, monospace` 与 `system-ui, -apple-system, sans-serif`。Fraunces 与 Inter 的三个 woff2 文件与对应 `@font-face` 一并停止引用（待删清单见 tasks 11 节）。
- 元素替换：页面背景是 24px 细网格；奖牌用 `[1]` `[2]` `[3]` 徽标（金 / 银 / 铜色边框），第 4 名起是 `#04` 这样的零填充序号，不再用 emoji；进度条换成分段 LED 条（`repeating-linear-gradient`）；今日时段分布换成分段方块柱、峰值琥珀色；用量趋势与名次走势用内联 SVG 折线（共用组件 `LbSparkline.vue`）。前三名的**判定**仍是 `entry.rank <= 3`，MUST NOT 按行下标或 `ordinal`——换的只是渲染形态，这条与 D16、与 `user-leaderboard` 的既有 Requirement 完全一致。
- 动效：v1 的条形生长、stagger 进场、`useCountUp` 数字滚动、`IntersectionObserver` 滚动进场与 `prefers-reduced-motion` 全局关闭全部保留，只把 gating class 随命名一起改成 `.gk-grown`、`.gk-stagger`、`.gk-reveal` / `.gk-revealed` 与 `--gk-bar-w`；`frontend/src/composables/useCountUp.ts` 原样复用，不改。环境动效从 aurora 换成 CRT 扫描线：页面根节点加 `.crt` 时叠加一层 `repeating-linear-gradient` 扫描线与标题光标闪烁，开关状态存 `localStorage['lb-crt']`，默认值仍取 `!prefers-reduced-motion`；`frontend/src/composables/useAmbientMotion.ts` 改名为 `useCrtEffect.ts`，模块级状态跨组件共享、读写包 `try/catch` 这两条实现约定不变。
- 顶栏换成状态栏。D15「页面自带顶栏、不套 `AppLayout`」不变，变的是它长什么样：左侧是 `sub2api://leaderboard` 与 `window=` / `metric=` / `mode=` / `snapshot=` 四个 chip，右侧是 `tz=` chip、CRT 开关、主题开关与 `← dashboard` 按钮。主题开关仍复用站点既有状态（`document.documentElement.classList.toggle('dark', ...)` + `localStorage['theme']`）。页面标题是一行命令 `$ leaderboard --window <w> --metric <m>` 带闪烁光标，`<w>` 与 `<m>` 取自当前请求参数；下一行是 `# N 位活跃 · 日期 · 档位说明`。
- 每个区块的标题也是命令：`$ top -n 50 --by total_tokens`、`$ stats --extremes`、`$ whoami --stats`、`$ models --today`、`$ profiles --top 8`、`$ platforms`、`$ activity --days 30`、`$ rhythm --weeks 4`、`$ trend --days 14`、`$ hours --today`、`$ tokens --composition`、`$ cache --trend 14`。命令与 flag 是 ASCII 字面量，MUST NOT 进 i18n——它们是「终端里敲的那行字」，翻译掉就不成立了；副标题的 `#` 注释、卡片标签、提示语与状态文案照常走 i18n，zh / en 齐备。页脚从叙事段落换成一行状态行。
- 非正常态（Preview / computing / stale / suppressed）沿用 v1 的判定字段与语义，只换皮：横幅是 `.card` 加一条琥珀左边条，骨架用 LED 条的样式。判定仍只依据 `preview` / `status` / `stale` / `entries_suppressed` 四个字段。
- 类名约定：token 文件只提供 `--gk-*` 变量与少量共享原子类，`gen_geek.py` 里为了生成静态画板而用的两字母类名（`.sb`、`.tr`、`.led`、`.tile` 之类）MUST NOT 原样搬进代码，各组件用 scoped 样式引用变量即可。mockup 是取值与版式的依据，不是类名的依据。
- 备选：沿用 v1 的 Editorial 皮肤。否决：用户看过第二轮 mockup 后选定了画布上的「Geek」那一页，旧 Editorial 画板明确留作对照、不再实现。
- 备选：两套皮肤做成可切换的主题。否决：理由与 D15 否决「侧边栏壳 + 内容区换皮」相同——两套视觉体系并存会互相打架，而且要为一张页面维护两份 token、两份组件样式与两份截图验收。
- 备选：奖牌继续用 🥇🥈🥉。否决：mockup 的徽标是 `[1]` / `#04` 两种等宽形态并存，emoji 在等宽表格里既对不齐也不是同一套视觉语言；判定规则不受影响，换的只是渲染。
- 备选：CRT 扫描线常开、不给开关。否决：与 v1 对环境动效的处理一致——持续动效必须集中在一个开关后面、默认值取 `!prefers-reduced-motion`、`localStorage` 持久化，低端设备与 reduced-motion 用户才有退路。
- 备选：改用 Google Fonts 引 JetBrains Mono 与 IBM Plex Sans。否决：D16 已经记过——CSP 没放行那两个域名，`@import` 会被静默拦截（页面不报错，字体直接不生效），为一张页面放宽全站 `style-src` / `font-src` 不划算。

### D20：用户维度的「之最」（Extremes）与聚合 SQL 的一次性扩容

- 四张 Highlights 之下加一排六张 Extremes 小卡，区块标题是 `$ stats --extremes`。每张卡的形状与 Highlights 一致：`{identity, ordinal, 该项的值}`。身份规则完全照搬 Highlights——`named` 档按 Named Participation 与 `username` 二次校验决定实名还是匿名，`anonymous` 档用该用户在当前 Window + Metric 榜单上的 Ordinal 假名，不在下发的 `entries` 里时 `ordinal` 为 `null`、前端渲染成「榜外用户」。六张卡全部按**当前 Window** 计算，在快照重建时算好，写进该 Window 的 highlights JSON 的 `extremes` 字段，因此与榜单同一批 `RENAME`、同一个 TTL、必然同源。
- 六项的口径：
  - `night_owl`（夜猫子）：该窗口内 0–6 点（站点时区）Total Tokens 最多者。`night_share_percent` 是其 0–6 点 tokens 占其自身该窗口 tokens 的整数百分比，两档都给；`night_tokens` 只在 `named` 档与 Preview 下给。
  - `rising`（进步之星）：**只在 `today` 窗口**存在。今日 Total Tokens 相对昨日增幅最大者，参评条件是昨日 tokens 大于 0 且今日大于昨日；值是 `change_percent`（整数，两档都给）。`week` / `month` 窗口下这张卡缺席（`rising` 为 `null`），页面不渲染它。
  - `omnivore`（杂食者）：该窗口内使用的不同模型数最多者，值是 `distinct_models`（两档都给）。
  - `talker`（话痨）：`output_tokens / total_tokens` 最高者，参评门槛是该窗口成功请求数不少于 5（常量 `leaderboardTalkerMinRequests`，与 `leaderboardCacheKingMinRequests` 同一类阈值）；值是 `output_share_percent`（整数，两档都给）。
  - `max_single`（单次最大）：单次请求 tokens（`input + output + cache_creation + cache_read`）最大者。`max_single_tokens` 只在 `named` 档与 Preview 下给；`ratio_to_median` 是其相对全体参与者「各自单次最大值」中位数的倍数（一位小数），两档都给。
  - `streak`（连续活跃）：截至今日或昨日、连续有用量天数最长者，值是 `days`（两档都给）。数据来自 `usage_dashboard_daily_users`，回溯 90 天，用 gaps-and-islands 的写法（按 `bucket_date` 减去行号得到分组键）一条 SQL 出结果。它与 Window 无关，三个窗口的这张卡内容相同。
- 聚合 SQL 在 D6「一条 SQL 同时产出三个窗口」的前提下扩容：每个窗口新增 `SUM(output_tokens)`、`SUM(cache_creation_tokens)`、`SUM(tokens) FILTER (0–6 点)`、`COUNT(DISTINCT model)`、`MAX(单次 tokens)` 与 `COUNT(*) FILTER (image_count > 0 OR video_count > 0)`（均带成功落账过滤）；另外加两个与窗口无关的「昨日」列 `yesterday_tokens` 与 `yesterday_requests`，扫描下界相应从 `min(月初, 周一)` 改成 `min(月初, 周一, 昨日起点)`。仍然是一条 SQL、一次扫描、同一个 `INNER JOIN users` 过滤。
- 每用户 Hash 的 value 从 4 段扩到 12 段，顺序固定为 `tokens, requests, input, cache_read, output, cache_creation, night_tokens, distinct_models, max_single, media_requests, yesterday_tokens, yesterday_requests`。解码沿用 D17 与 Migration Plan 第 6 条已经定下的规则：按逗号切分后逐段取值，**段数不足时缺的段一律按 0**，因此上一版写入的 4 段式 value 仍能读（表现为该窗口缺对应的之最卡，而不是把 0 当成真值），下一轮作业最长 5 分钟后补齐；首两段解析失败仍视为无效行。ZSET 仍然只有两个 Metric，新增的八个数一律不参与排名。（D24 把 value 再扩到 13 段、第 13 段是 Cost micros，并因此把每个 Window 的 ZSET 增到三个；「缺段按 0」这条规则原样适用，12 段式旧值解出 `CostMicros = 0`。）
- `media_requests` 是「有 `image_count > 0` 或 `video_count > 0` 的成功请求数」，本轮**只存不展示**：它已经占住 Hash 的第 10 段与聚合 SQL 的一列，但响应里没有对应字段，页面上也没有对应卡片。
- 备选：为新增的八个数另起一个 Hash，或把 Hash value 换成 JSON。否决：段数扩展已经有既定且验证过的向后兼容写法（缺段按 0，见 Migration Plan 第 6 条），换结构反而要同时处理两种编码并存；JSON 还会让每个用户的 value 从几十字节涨到几百字节。
- 备选：`rising` 在 `week` / `month` 窗口也算，与上周 / 上月比。否决：本轮只往聚合里加了「昨日」一组基线列，没有同口径的「上周 / 上月」；为两个窗口各加一组基线要再扩两列并把扫描下界推到上月初，代价与一张卡不成比例。
- 备选：`max_single` 在 `anonymous` 档也给绝对 tokens。否决：D18 的规则是任何他人的绝对量在该档一律缺席，单次最大 tokens 是不折不扣的绝对量；改用相对中位数的倍数表达同一件事。
- 备选：`streak` 回去扫 `usage_logs`，或按 Window 各算一份。否决：`usage_dashboard_daily_users` 就是为「某人某天有没有用量」准备的，主键即索引；连续活跃天数本身跨窗口，按窗口切反而算不出「连续 23 天」这种事实。
- 备选：`media_requests` 这一轮就出一张卡。否决：多模态请求在多数站点上还没有量，一张常年为空的卡不如先把数存下来，等有数据再出（属后续增量）。

### D21：Viewer Stats 与名次历史表，`viewer.models` 是 D8「请求路径只读」的唯一例外

- 「你的位置」之后、榜单之前加一块 `$ whoami --stats`，只给本人看，响应里是新的顶层字段 `viewer`。它有四部分：名次走势 `rank_history`、本人模型偏好 `models`、本人缓存命中率 `cache_hit_rate`、本人平均每请求 tokens `avg_tokens_per_request`。`viewer` 里全是**查看者自己的**数据，因此任何档位下都是真实值，不受 D18 的裁剪影响——这与「本人条目与 `my_rank` 始终真实」是同一条既有规则。
- `rank_history` 是近 14 天每天在 `today` 窗口按 Total Tokens 的名次，形如 `[{date, rank}]`，前端画折线（名次越小画得越高）并给一句「从 #a 到 #b · 最好 #c」。它需要逐日留痕，因此新建表 `leaderboard_rank_history(user_id BIGINT, snapshot_date DATE, rank_total_tokens INT, rank_successful_requests INT, PRIMARY KEY (user_id, snapshot_date))`，迁移编号 `239`（见 Migration Plan）。（D24 加第三列 `rank_cost`，迁移 `241`；页面上的「近 N 天名次」折线仍按 Total Tokens 的名次画，cost 名次只是一并攒起来。）快照作业每一轮对 `today` 窗口的**全部参与者**做一次 upsert（多行 `INSERT ... ON CONFLICT (user_id, snapshot_date) DO UPDATE`，每批 1000 行），当天的多轮互相覆盖，日终那一轮写下的就是当天的最终名次。同一轮里顺带 `DELETE FROM leaderboard_rank_history WHERE snapshot_date < 今日 - 90 天`，保留期 90 天，与 `rank_history` 只取 14 天留足余量。两个名次列都写，是因为它们本来就在同一份 Snapshot 里、写第二列不多一次计算，而只写一列会让「按成功请求数看走势」这种后续增量必须重新攒历史。
- `models` 是本人在**当前 Window** 的 Top 5 模型及占比。这一项做不到只读 Snapshot：按用户 × 模型的聚合如果要预先算好，就得给全站每个人每个窗口各存一份，聚合成本与用户数成正比，直接顶掉 Goals 里「聚合成本与在线用户数无关」的前提。因此在 D8 上开一个**唯一的例外**：请求路径允许一条 `WHERE user_id = 查看者` 且限定窗口区间的 `usage_logs` 聚合（`GROUP BY model`，走 `(user_id, created_at)` 索引，只扫这一个人的行）。例外的边界写死三条：只查查看者自己、只在这一个字段上、结果在 Redis 上按 `(user_id, window, 窗口起点)` 缓存 60 秒。除此之外请求路径对 `usage_logs` 的查询次数仍然是 0。
- `cache_hit_rate` 与 `avg_tokens_per_request`（= 本人该窗口 tokens / 成功请求数）来自本人在该 Window Hash 里的那一行，不额外查库。页面把它们与全站值并排对比，全站值取 `highlights.site.cache_hit_rate` 与新增的 `highlights.site.avg_tokens_per_request`——两者都是比率，按 D18 两档都下发。
- `viewer` 不随 `status` 变化：`rank_history` 来自 Postgres、`models` 来自那条例外查询，两者都不依赖 Snapshot，因此 `status` 为 `computing` 时 `viewer` 照常返回。查看者没有名次历史时 `rank_history` 是空数组，该窗口零用量时 `models` 是空数组、两个比率字段缺席；`viewer` 本身 MUST NOT 是 `null`。
- 提示语（`my_rank.hint`）沿用 v1 的结构化字段与按档位选文案的做法，不改。
- 备选：`viewer.models` 也放进后台作业预先算好。否决：见上——那是「全站每人每窗口一份」，与本变更「聚合成本与在线用户数无关、全站一份」的前提直接冲突。
- 备选：不建表，名次走势在请求时现算。否决：历史名次要靠历史 Snapshot，而 Snapshot 的 key 随窗口自然过期、只留当前窗口一份；现算等于回去扫 `usage_logs` 三十天，是 D8 明令禁止的。
- 备选：名次历史每天只在日终写一次。否决：那要另起一个定时点（多一处可能停摆的地方），而且当天之内页面上根本没有任何名次点可画；每轮覆盖用的是已经算好的排名，额外成本只有一次批量 upsert。
- 备选：名次历史不做清理。否决：这张表按「人 × 天」增长，没有保留期就会无限长；90 天与 `streak` 的回溯窗口一致，一个数管两处。

### D22：新增的五块站点级 Insights 与它们各自的来源

- 五块新区块与既有的 `models_today` / `daily_30` / `hourly_today` / `cache_today` / `month` 并列挂在 `insights` 下，同样在 5 分钟作业的同一轮里算好，请求路径仍然只读 Redis（唯一例外是 D21 的 `viewer.models`）。既有五块的口径、字段与降级规则一个字不改。
  - `profiles`（模型偏好画像）：当前 Window 按 Total Tokens 的前 50 名（`leaderboardProfilesLimit = LeaderboardTopEntryLimit`，与榜单本体同一个数），各自在该窗口用过的全部模型及占比（占比是该模型占本人成功请求的整数百分比，按成功请求降序，不截断）。一条按 `(user_id, model)` 的聚合、`user_id` 限定在这 50 个之内。名字按档位渲染（规则与榜单条目、Highlights 完全相同），占比两档都给。它是**按 Window** 的，因此存在该 Window 的 highlights JSON 里（与 `extremes` 同一个 key、同一批 `RENAME`），下发时挂在 `insights.profiles` 下——存储按「重建时按什么分组」划分，下发按「页面上属于哪个区块」划分。`status` 为 `computing` 时它随 highlights 一起为 `null`。
  - `platforms_today`（平台分布，`$ platforms`）：今日的成功请求 `INNER JOIN accounts` 后按 `platform` 聚合，形如 `[{platform, share_percent, successful_requests?}]`。`share_percent` 两档都给，`successful_requests` 只在 `named` 档与 Preview 下给。用 `INNER JOIN` 而不是渠道监控那样的 `LEFT JOIN`，与 D6 对孤儿日志的处理一致：账号已被删掉的日志没有平台可归，不该凑出一个空平台分类。
  - `weekly_rhythm`（周内节奏，`$ rhythm --weeks 4`）：近 28 天的 `usage_dashboard_hourly` 按 `(weekday, hour)` 求平均请求数，输出 7 × 24 的 0–4 等级（相对这 28 天里最大值分五档），两档完全相同。某个 `(weekday, hour)` 在表里没有行，意思是「那个时段这 28 天没有请求」，等级为 0；整张表在这段区间里一行都没有时，整块为 `null`（与其它预聚合区块同一条降级规则）。
  - `composition_today`（Token 构成，`$ tokens --composition`）：今日 `input` / `output` / `cache_creation` / `cache_read` 四段的占比与绝对量，来源是 `today` 窗口聚合的站点合计（D20 已经把 output 与 cache_creation 加进那条 SQL，因此不必另查）。四个 `*_percent` 两档都给，四个绝对量只在 `named` 档与 Preview 下给。
  - `cache_trend_14`（缓存命中率趋势，`$ cache --trend 14`）：近 14 天每天的 `cache_read / (input + cache_read)`，来自 `usage_dashboard_daily`，形如 `[{date, cache_hit_rate}]`。命中率是比率，两档都给。
- `highlights.site` 增一个 `avg_tokens_per_request`（全站该窗口 tokens / 成功请求数）。它是比率，两档都下发；全站成功请求数为 0 时该字段缺席，MUST NOT 记成 0，与 `cache_hit_rate` 同一条写法。
- 降级：来自预聚合表的 `weekly_rhythm` 与 `cache_trend_14` 沿用「缺行即整块 `null`、前端隐藏该区块」的既有规则，MUST NOT 用 0 填充，也 MUST NOT 回落到实时扫 `usage_logs`；`profiles`、`platforms_today` 与 `composition_today` 不依赖预聚合表，因此关掉 `cfg.DashboardAgg.Enabled` 不影响它们。作业里这五块的失败与既有 insights 一样只降级为 `null` 并记日志，硬失败仍然只有榜单本体与 `models_today` 两条。
- 备选：新增区块在请求路径上现算。否决：D8 与 D17 已经定了请求路径只读，`profiles` 要按用户 × 模型聚合、`platforms_today` 要 `JOIN accounts` 扫今日，两条都是明令禁止的；唯一的例外只开给 D21 的 `viewer.models`，且限定「只查自己」。
- 备选：`profiles` 按 Window 各存一个独立的 key。否决：Window 级的 JSON 已经有 highlights 这一个 key，再加一个只是把同一轮写入、同一批 `RENAME`、同一个 TTL 的东西拆成两处，还要多一次读。
- 备选：`weekly_rhythm` 下发平均请求数本身。否决：那是站点级绝对量，`anonymous` 档一律不下发；热力图要的本来就是色阶，0–4 等级两档通用，也省掉「同一块数据两种形态」的分支。
- 备选：`platforms_today` 与 `composition_today` 在 `anonymous` 档整块缺席。否决：D18 已经否决过「`anonymous` 档干脆不给 Insights」——相对量与比率本来就不泄露绝对规模，整块拿掉会让匿名档的页面塌掉一半。


### D23：报表皮肤（2026-09-12，mockup 已确认），取代 D19 的极客皮肤

> **已被 2026-09-14 的 spool 内页皮肤取代**（用户看过 `scratchpad/lb-proto/final/` 原型后拍板，落地契约见 `scratchpad/lb-proto/LANDING-CONTRACT.md`，任务条目见 `tasks.md` 第 16、17 节）。报表皮肤的具体形态——**亮色为基色 + `.rp.dark` 两套配色**、明暗跟随 `html.dark` 并由本页主题开关同步、Geist / Geist Mono 自托管字体、零阴影的纸感取值，以及那段章序（01 今日高亮 / 02 六个之最 / 03 我的位置 / 04 榜单 Top 50）——不再是实现依据，本条保留作决策沿革，不删。新皮肤是**深色专属**（`.rp` 上直接定义深色变量，页面不提供主题切换，也不读写站点的 `html.dark` / `localStorage['theme']`），字体改用系统栈不再加载任何字体文件，章序见本文件末尾的七章↔组件映射表。仍然成立并由新皮肤原样承接的是四条约束：页面根节点仍是 `.rp` 且变量定义在它这一层而不是 `:root`、样式表仍只由 `LeaderboardView.vue` 自己 `import` 不进全局入口、既有 `rp-` 类名与全部 `data-testid` 保留（换皮只改 CSS 取值）、前三名徽章按条目的 `rank` 值判定而不是行下标或 `ordinal`。

> **2026-09-14 再修订**（用户看过线上后的第三条反馈：「没有做主题适配」「这里的国际化也有问题」，任务条目见 `tasks.md` 第 18 节）。上一段里「新皮肤是**深色专属**（…页面不提供主题切换，也不读写站点的 `html.dark` / `localStorage['theme']`）」这半句的「深色专属」**MUST NOT 再作为实现依据**，保留作决策沿革，不删；「不提供页内主题切换、不读写站点主题状态」照旧成立。现状：
> - 明暗重新跟随站点的 `html.dark`，但方向与机制都与 v3 不同。v3 以亮色为基色，由 `LeaderboardView.vue` 按 `document.documentElement.classList.contains('dark')` 算出 `.rp.dark` 修饰类（页面持有 JS 主题状态，并由本页开关同步站点主题）；现在以暗色为基色，暗色变量仍定义在 `.rp` 上（已验收的取值原样保留），亮色由样式表里紧随其后的 `html:not(.dark) .rp` 覆写同名变量——纯 CSS 推导，根节点恒为 `.rp`，页面零 JS 主题状态，开关只在站点侧边栏。
> - 先把样式表里散落的颜色字面量（分段 / chip / 热力零值 / 网格线 / 光晕 / 条形轨道 / 骨架 / 悬停 / 本人行淡底等，全是按深底推导的 rgba）逐个提成语义变量，暗色值原样搬进 `.rp`，亮色值写进覆写块；两个变量块之外不再有颜色字面量（`transparent` 除外），新增颜色必须两个块都给值。
> - 亮色取值照站点亮色外壳而不是把暗色反色：面板底 `#f9fafb`（与 main 底 gray-50 同值）、浮层（卡片 / 窗口内容 / 控件）白、边框 gray-200、标题栏 gray-100；中性色用 Tailwind 默认 gray，承载数据的灰字 gray-600（gray-500 在 gray-100 标题栏上不到 4.5:1）；大标题色 `--cream` 改成 gray-900。赤陶与琥珀在亮色下拆成两档：文字档在原色相上加深到 ≥4.5:1（赤陶 `#a64526`、琥珀 `#986206`），图形档只要 ≥3:1（赤陶 `#d66b48`、琥珀 `#bf7b08`，对白与对条形轨道都过 3:1），不借用站点的 teal、不引入新色相。条形轨道 `--track` 在亮色下 MUST 是不透明色（`#eeeff0`，即 slate-900 .07 叠白的结果）：写成半透明时它会叠在行底色上，悬停的前三名行与本人行里图形档对轨道会掉到 2.67–2.87:1。承载数据的文字只有一处例外：64–80px 粗体的名次大字 `.rp-rank-v` 用图形档（对白 3.47:1，按 WCAG 大字档 3:1），否则亮色下赤陶→琥珀的渐变看不出来。逐项对比度与不追 3:1 的项（与暗色同档或纯装饰）写在 `leaderboard-tokens.css` 亮色块表头的注释里。
> - 备选：沿用 v3 的 JS 推导（`isDark` ref + 修饰类）。否决：页面会重新持有一份主题状态，要监听站点在侧边栏里的切换才能同步，而 CSS 选择器天然跟随 `html.dark`、零同步成本。备选：按 `prefers-color-scheme` 判定。否决：理由同 D16——与站点开关不同步。
> - 榜单窗口标题栏只留三个圆点 + 刷新按钮，不放任何文字（2026-09-14 最终做法）。左侧上一轮自造的 `usage.board` 是没翻译的英文；右侧状态小字 `<window> · <metric> · <top N>` 与 `leaderboard-board-meta` 一度按「全部 data-testid 不动」恢复并做了 i18n，随后整条删除：它把正上方页头控制条已高亮的窗口与指标、章名里已写的「前 50 名」再印一遍，与用户一贯的「不够紧凑」「去重复」相反；用户接着要求「顶部也做成中文」，页头的 Window / Metric / Snapshot 标签一律走 i18n（见 spec 豁免清单的修订），状态小字已无存在理由。否决：保留 i18n 化的状态小字（重复）；删左侧留右侧（同上）。

**视觉体系（取代 D19 的 `.gk`）**

- 页面根节点 class 从 `.gk` 改为 `.rp`（report），变量前缀从 `--gk-*` 改为 `gen_taste.py` 里那套无前缀名。作用域规则不变：变量仍定义在页面根节点这一层而不是 `:root`，仍只由 `frontend/src/views/user/LeaderboardView.vue` 自己 `import`，不进全局样式入口。`frontend/src/styles/leaderboard-tokens.css` 整份替换——所有 `.gk*` 选择器、`--gk-*` 变量、CRT 扫描线、光标闪烁与网格背景规则一并删除，MUST NOT 留兼容层或别名。
- 明暗方向再翻一次：报表皮肤以**亮色为基色**（变量定义在 `.rp` 上），站点处于暗色时页面根节点追加 `dark`，由 `.rp.dark` 覆写同名变量。这个 class 仍由 `LeaderboardView.vue` 按 `document.documentElement.classList.contains('dark')` 推导（沿用 v2 的 `isDark` 逻辑，只是取反方向反过来），并在本页主题开关切换时同步更新。D16 与 D19 的约束不变：不另起一套页面私有的主题状态，也不用 `prefers-color-scheme` 单独判定。
- CSS 变量名与色值照 `gen_taste.py` 的 `CSS` 常量原样移植：`--pad`、`--rail`、`--rail-gap`、`--mono`、`--bg`、`--bg-sunk`、`--band`、`--panel`、`--ink`、`--ink-2`、`--ink-3`、`--ink-4`、`--line`、`--line-soft`、`--line-hard`、`--dot`、`--heat-0`、`--accent`、`--accent-deep`、`--accent-a` / `-b` / `-c` / `-d`、`--accent-wash`、`--accent-edge`。亮暗两套色值照抄，唯一强调色是亮 `#a8412b` / 暗 `#e08066`，中性色是冷 zinc 系；MUST NOT 出现 `#000` 或任何 `rgba(0, 0, 0, …)`。
- 字体换成 **Geist（正文与标题）+ Geist Mono（所有数字，配 `font-variant-numeric: tabular-nums`）**，两款都自托管：`v3/geist-sans.woff2` 与 `v3/geist-mono.woff2` 复制为 `frontend/src/assets/fonts/leaderboard/geist-sans-latin.woff2` 与 `geist-mono-latin.woff2`（可变字重 100–900，拉丁子集，来源 fontsource，OFL）。CSP 的结论与 D16、D19 一致（`security_headers.go` 没放行 `fonts.googleapis.com` / `fonts.gstatic.com`，`@import` 会被静默拦截），因此 `@font-face` 同样只写在 `leaderboard-tokens.css` 里、配 `font-display: swap`；中文与兜底栈是 `system-ui, -apple-system, "PingFang SC", "Noto Sans CJK SC", sans-serif`，等宽兜底是 `ui-monospace, SFMono-Regular, Menlo, monospace`。JetBrains Mono 与 IBM Plex Sans 的两个 woff2 停止引用并进待删清单。
- 材质禁令（来自用户安装的 `design-taste-frontend-v1` skill）：页面上零 `box-shadow`、零 `backdrop-filter`、零 `filter`、零渐变（`repeating-linear-gradient` 的 LED 条随之消失）、零 `z-index`（除非某个浮层确实必须）。圆角只有两档：控件 4–5px、条与热力格 1.5–2px。层次全部靠 1px `--line` 分隔线、`--bg-sunk` 底色与字重字号拉开，不靠阴影。
- 图标新增 `frontend/src/components/user/leaderboard/LbIcon.vue`：一个 `name` prop，内联 `gen_taste.py` 的 `ICONS` 字典路径（`arrow-left`、`sun`、`moon`、`refresh`、`eye-off`、`clock`、`rows`、`layers`、`pulse`、`calendar`、`bars`、`crown`），`viewBox="0 0 16 16"`、`fill="none"`、`stroke="currentColor"`、`stroke-width` 1.35–1.5、`aria-hidden="true"`。页面上 MUST NOT 再出现 unicode 符号当图标（☼ ☾ ▤ ↻ ▲ ▶ 之类全部换成 `LbIcon`），也 MUST NOT 引入 emoji。

**页面骨架：masthead + 标题块 + 七章（取代 D19 的状态栏 + 命令行标题）**

- **Masthead**（新组件 `LbMasthead.vue`，取代 `LbStatusBar.vue`）：**一行两端对齐、高度收紧、底部一根 `--line-hard`**（2026-09-12 晚按用户目视反馈收口）。左侧刊头只有一行：粗体站点名 + 一个细字「用量排行榜 / Leaderboard」；右侧一行放窗口分段（today / week / month）、指标分段（tokens / 请求数 / 金额，第三项由 D24 加入）、匿名档时的一个小 chip「匿名档」（实名档与档位未知时不挂任何档位 chip）、快照 chip（呼吸点 + `HH:MM` + `（+Nm 后重建）`）、主题分段（亮 / 暗，用 `aria-pressed` 表达选中）与「← 返回仪表盘」。`LEADERBOARD` 小字、日期行、`HH:MM · <tz>` 行以及 `WINDOW` / `METRIC` 两个键名、`MODE named` chip、`TZ <站点时区>` chip 全部删除——日期进标题块副题、时区进页脚、快照时分进那个 chip，同一个事实在页面上只出现一次。窗口与指标的切换在 masthead 与榜单章的工具条**两处都有**，两处共用同一份状态、同一个请求参数。`(+Nm)` 后缀沿用 v2 在 `LbStatusBar` 里的规则：按重建周期现算，pending / stale / 超过一个周期时不渲染。≤767px 时左块与右块各占一行。
- **标题块**（新组件 `LbTitle.vue`，取代 `LbPrompt.vue`）：H1 随 Window 变化——`今天谁在用` / `本周谁在用` / `本月谁在用`，字号 28px，不做巨型 hero、不居中；`· today` 这段窗口回显已删（窗口字面量就印在报头的分段上）。副题只剩两段：`7 位活跃 · 2026-09-12`（匿名档是 `5+ 位活跃 · 2026-09-12`）。档位说明三句与「全站与他人只按口径聚合，不展示金额、邮箱与用户 ID」那一句都已删——档位在匿名档由报头的 chip 表达，隐私声明页脚已有一份。非 ready 时不渲染人数，沿用 v2 的 `ready` 判定。
- **章节容器**（新组件 `LbChapter.vue`）：通栏；章名行是「章号（等宽、强调色）内联在章名前 + 右侧工具条」，下面是内容 slot；章与章之间用 1px `--line` 分隔。画板上的左侧边注栏（章号 + 口径说明 + 标签）在用户目视后被判「太冗余」，2026-09-12 晚分两步去掉：先删口径说明，再整栏去掉；同一轮里章名右侧那句小字副题（`chapters.NN.sub`）也整体去掉，`subtitle` 这个 prop 保留但页面上零传值，只有 04 章那一格让位给榜单工具条。章名文案走 i18n。
- **章号是固定编号，不是序号**：某章因数据缺失整章不渲染时（典型是 05/06/07 这些只依赖 `insights` 的章：来源表缺行时后端给 null），其余章的章号 MUST NOT 重排——页面上出现 `01 02 04 05 06 07` 是正确行为。这条是报表皮肤独有的新约束，v2 的区块没有编号，不存在这个问题。
- 七章与组件的映射（组件名沿用 v2，内部全部换皮，`.gk-*` class 全部改 `.rp-*`）。**章序已于 2026-09-14 的 spool 内页皮肤重排**为「01 排行榜（前 50 名）／02 你的排名／03 今日亮点／04 六项纪录／05 模型与平台／06 活跃节奏／07 趋势与构成」——榜单提到页头正下方，D23 原先写的「01 今日高亮／02 六个之最／03 我的位置／04 榜单 Top 50」这段章序 MUST NOT 再作为实现依据；各章的组件构成、口径与内部结构照旧：
  - **01 排行榜（前 50 名）** = `LbRankList.vue`。窗口 / 指标的分段已于 2026-09-14 的 spool 内页皮肤移进页头一处，本章右上角的工具条整条去掉，刷新 `LbIcon` 移进榜单窗口的标题栏；表头是 `名次 / 用户 / 相对第一名 / TOKENS / 成功请求 / 金额`（末列由 D24 加入，右对齐、按查看者 locale 用 `formatCurrency` 渲染）；名次补零成 `01`；本人行左侧 3px 强调色边条加淡底加「你」标记；相对第一名画成细条加百分比。表下只剩**一行解读句**，由前端从响应数据现算，两档同形：`第 1 名是第 2 名的 1.7 倍 · 前三名占全站 88%`，缺数据的分句整句省略（两句都缺时整行不渲染），分句不带句末标点、由前端用 ` · ` 连起来。两个分句都按当前 Metric 现算，`metric=cost` 时分母取 `highlights.site.cost`（D24）。第 1 名的绝对量、「是末名的几倍」、相邻两名在另一个 Metric 下互换那句，以及整段注脚（「共 N 位活跃…」「竞赛排名…」「本人行带…」）全部删除——前者表格第一行已经印着，后者说的是表格自己已经画出来的东西。
  - **02 你的排名** = `LbWhoami.vue`。两栏 `.82fr 1.8fr`：左栏是 `#1` 大数字、`参与人数 N 人 · 相对第一名 N%`、一个「仅本人可见」的私密标记与模型偏好 chip；右栏是「近 N 天名次」（N 取折线实际点数）的折线（复用 `LbSparkline.vue`，纵轴反转；坐标轴说明那句已删）、下方只剩 `最好 #a · 最差 #b`，再下方是「我 VS 全站」两组双条（缓存命中率、平均每请求）。「已进前 10，你在榜单里高亮显示」那一句提示删掉（名次那个大数字已经说完），「只有你自己能看到这一块」压成一个极小的「仅本人可见」标签，全页只留这一处。只要拿到响应就渲染本章（含 `computing` 与零用量态：零用量时名次格显示「本窗口暂无用量」，与 user-leaderboard spec 的 My Rank 要求一致）；`rank_history` 为空时右栏折线不渲染、两栏收成单栏（`.is-single`）。章号固定不重排。
  - **03 今日亮点**（week / month 下是「本周亮点」/「本月亮点」）= `LbHighlights.vue`。三栏 `1.55fr 1fr 1.12fr`：左栏「卷王」大数字加与第 2 名的对比双条，配一句现算的对比（`比第 2 名多 <差值>（领先 N%），占全站当日 N%`，差值是 top1 与 top2 的 `total_tokens` 之差，只在 `named` 档与 Preview 下出现）；中栏上半是「效率之星」（缓存命中率大数字 + 用户 + `在 <模型> 上最有效 · 需成功请求 > 5 次`），下半是「最勤快」（请求数大数字 + 用户 + `占全站 N% · 与第 2 名并列 / 领先 N%`）；右栏是「全站今日」的引导点线五行（总 tokens、成功请求、活跃人数、峰值时段、缓存命中率），「站点级聚合，不落到任何个人」那行小字已删。卷王那句现算的对比句两档同形，压成 `领先第 2 名 66% · 占全站 41%`（并列时换成 `与第 2 名并列 · 占全站 41%`）；效率之星下方那一行压成 `最有效：<模型>`；最勤快下方保留 `占全站 21% · 与第 2 名并列 / 领先 N%`。`anonymous` 档下右栏只剩活跃人数（带 `5+`）、峰值时段与缓存命中率，没有绝对量的行整行不渲染。
  - **04 六项纪录** = `LbExtremes.vue`。三列两行的 hairline 网格 `1.18fr 1fr 1.26fr`，每格是小标题、大数字与单位同行、强调色用户名一行（「单次最大」的用户名后保留 `· 中位数的 N 倍`，`· 只在 today 窗口` 那句注脚已删——进步之星本来就只在 today 窗口才有格子，缺席即是说明）；缺席的项整格不渲染（`rising` 在 week / month 必然缺席），序列用 `margin-top: calc(var(--i) * 4px)` 做阶梯位移，因此缺项时后面的格子自动前移。
  - **05 模型与平台** = `LbModelHeat.vue` + `LbProfiles.vue` + `LbPlatforms.vue`。上半是 `1.32fr 1fr` 两栏（左今日模型条、右画像 chip 表，chip 左边界对齐成一条轴），下半是通栏的平台堆叠条加图例；图例最后一项「其他 N%」只在各项之和小于 100 时出现，且 MUST NOT 为它编一个请求数（`· 取整残差` 那句解释已删）。模型块的标签是「今日模型」，画像块是「模型偏好 · 按成功请求占比」。
  - **06 活跃节奏** = `LbHeatmap.vue` + `LbRhythm.vue` + `LbHourly.vue`。顶部通栏是 30 天条形热力（一行 30 格加起止日期刻度）；下方 `1.42fr 1fr`：左是 7 × 24 周内节奏（标签是「周内节奏 · 近 4 周」，周一起算，四级明度加「少…多」图例），右是今日时段柱加**唯一一句** `峰值 HH:00 · N 次`。「主要落在 HH:00 — HH:00」那半句、整段现算的节奏说明（最密的两个时段 / 最深的一格 / 有用量的天数）以及两处「计数含失败请求的占位记录…」的口径注脚都已删——柱形与格子本身已经把这些画出来了，口径差异留在 `CONTEXT.md` 词条里。
  - **07 趋势与构成** = `LbTrend.vue` + `LbComposition.vue` + `LbCacheTrend.vue`。上半 `3.3fr 1fr`：左是 14 天柱，右是「本月累计」大数字加「较前一日 +N%」；下半是「今日 TOKEN 构成」四段堆叠条加图例、「缓存命中率 · 近 14 天」大数字加折线加一个「今日」标签（`· 区间 a–b%` 已删，区间由右边的折线自己说）。
- **断轴规则**（07 章的 14 天柱，报表皮肤新增）：取这 14 天里第三大的值 `T`，若最大值大于 `5T` 则纵轴在 `T` 处压缩——`T` 以下占柱区 66% 高、`T` 以上占 22%、中间 12% 画断轴虚线并标注「轴在 <T> 压缩」；否则线性、不画断轴。图下那句「最高一天是第三高那天的 N 倍…」的说明已删，标注本身就说明轴被压缩了。阈值与比例是固定常量，MUST NOT 随数据自适应，也 MUST NOT 在不满足条件时仍画断轴——断轴只在它确实发生时出现，否则读者会以为轴一直是断的。
- **页脚**（`LbFooter.vue` 换皮）：一行四段 `snapshot HH:MM · 每 5 分钟重建 · <tz> · 不展示金额与邮箱`（D24 之后金额是可展示的量，这一段只剩邮箱那一句；i18n 键名 `leaderboard.footer.noMoney` 沿用不改，`LbFooter.vue` 本身不动）。`COLOPHON` 小字、`一周从周一起算` 与 `successful_requests = actual_cost > 0` 已删；`snapshot` 与时区名是字面量不进 i18n，「每 5 分钟重建」与隐私声明走 i18n。
- **非正常态**（`LbStates.vue` 换皮）：加载 / 计算中 / 失败 / 空 / 抑制的判定字段与文案一个字不改，只换皮——骨架用 `--bg-sunk` 色块，MUST NOT 用圆形 spinner；做 shimmer 的话只动 `transform`。

**动效**

- 只动 `transform` 的三条入场：`rise`（`translateY(7px)` → `none`）、`grow`（`scaleX(.88)` → `none`，用于横条）、`growy`（`scaleY(.9)` → `none`，用于竖柱）。缓动统一 `cubic-bezier(.16, 1, .3, 1)`，时长 0.52–0.6s，级联 `animation-delay: calc(var(--i) * 60ms)`（条与柱另加 80ms），`--i` 由模板上的 `:style="{ '--i': i }"` 给出。
- **MUST NOT 用 opacity 做入场**：首帧就要可读，文字不能从透明淡入。这条同时废掉 v2 的 `.gk-reveal` / `IntersectionObserver` 滚动进场机制——v3 用纯 CSS 级联，不依赖元素是否进入视口，因此首屏之外的内容在截图与打印里也是完整的。
- 常驻微动只剩 masthead 上的快照呼吸点（`breathe 2.8s ease-in-out infinite`，只动 `opacity` 的一个 5px 圆点）。所有可点元素的 `:active` 统一 `translateY(1px) scale(.985)`。
- `prefers-reduced-motion: reduce` 时 `.rp` 下所有 `animation` 与 `transition` 一律 `none !important`，数字不滚动（`useCountUp` 已经支持，原样复用不改）。
- **CRT 整条链删除**：`frontend/src/composables/useCrtEffect.ts` 及其 spec、`localStorage['lb-crt']` 键、扫描线层与光标元素全部删除，页面上不再有任何「环境动效开关」。D19 为 CRT 定的那条「持续动效必须放在开关后面」在 v3 下自然满足：没有持续动效可开关。

**断点**

- ≤1180px：三栏收成两栏（照画板的 `@media (max-width: 1180px)` 规则）。
- ≤767px：一律单列；masthead 换行成两行；榜单改两行式（第一行名次 · 用户 · tokens · 金额，第二行相对条 · 百分比 · 请求数，三个数值格一律靠右，相对条 MUST NOT 被隐藏）；7 × 24 网格的轨道一律 `minmax(0, 1fr)` 自适应收窄。任何宽度下页面 MUST NOT 出现横向滚动（表格、7 × 24 网格这类必须更宽的东西各自套自己的 `overflow-x: auto` 容器）。

**档位与隐私（与 D1、D18、D20 一致，此处重申）**

- 实名档：开着昵称展示的用户显示 username，其余是「第 N 位」；本人行是本人的 username（缺席时「当前用户」）加「你」标记。匿名档：他人一律「第 N 位」，他人与站点级只有占比、相对值与倍数，参与人数带 `5+` 分档；本人行与 03 章的本人统计保留真实数值；「杂食者」的「N 种模型」是类别数不是绝对量，两档都给。任何档位下都 MUST NOT 出现邮箱与 `user_id`；金额只有 Cost 一个口径，与 tokens 同一套档位规则（D24）。
- 解读句、节奏说明、对比句这类**现算文案** MUST 全部由前端从响应数据算出，缺哪个数就省哪个分句，MUST NOT 把 mockup 里的示例数字写死进模板——画板上的 `4.37M`、`+241%`、`28 天` 全是假数据。

**i18n 命名**

- 新增 `leaderboard.masthead.*`、`leaderboard.titleBlock.*`、`leaderboard.chapters.01.*` – `leaderboard.chapters.07.*`（只剩 `name`，01 的 `name` 随 Window 有三份）、`leaderboard.rank.readout.*` 与 `leaderboard.insights.trend.axisBreak`；删除 `leaderboard.statusBar.*`（CRT 两条文案在里面）与 `leaderboard.prompt.*`，以及换皮后零引用的其它旧键。2026-09-12 晚的文案瘦身又删掉一批零引用键：`masthead.window` / `metric` / `mode` / `tz` / `snapshot`、`titleBlock.ruleNamed` / `ruleAnonymous` / `rulePreview` / `siteRule`、六处 `chapters.NN.sub`、`extremes.rising.scope`、`whoami.rankAxisHint`、`highlights.leadSayNamed` / `leadSayAnonymous` / `siteNote`、`rank.readout` 里除 `lead` / `topThreeShare` 之外的九条、`myRank.hint.inTop`、`insights.preaggregateNote`、`insights.trend.axisBreak.note`、`insights.hourly.sub`、`platforms.otherNote`、整块 `rhythm.readout`、`footer.weekStart`；新增 `masthead.modeAnonymous`、`highlights.leadSay` 与 `footer.rebuild`。zh / en 两侧 schema 必须对齐，页面上 MUST NOT 留零引用键。
- 标题块的键取 `titleBlock` 而不是增补文件里写的 `title`：`leaderboard.title` 已经是一个叶子字符串，`frontend/src/router/index.ts:253` 用 `titleKey: 'leaderboard.title'` 把它当路由标题读，改成对象会连带打断路由标题与面包屑。这是本轮唯一一处对增补文件命名的偏离，理由写在这里。
- 断轴说明挂在 `leaderboard.insights.trend.axisBreak` 而不是新开一个顶层 `leaderboard.trend`：`LbTrend.vue` 现有文案就在 `leaderboard.insights.trend.*` 下，新开一个只会让同一个组件读两处。

**备选与否决**

- 备选：沿用 v2 的 Geek 皮肤。否决：用户装了 taste skill 之后按它的禁令清单重出了第三版 mockup 并确认了「报表」这一版，Geek 画板与 Editorial 画板一样留作对照、不再实现。
- 备选：两套皮肤做成可切换主题。否决：理由与 D19 否决同一提议时相同——两套视觉体系并存要维护两份 token、两份组件样式与两份截图验收，而且本轮的骨架（七章）与 v2 的骨架根本不是同一棵 DOM，切换不可能只切样式。
- 备选：保留 `.gk-*` 类名与 `--gk-*` 变量做别名，减少 diff。否决：别名层会让两套命名同时存在于一份 token 文件里，下一次换皮时没人敢删；`leaderboard-tokens.css` 本来就只被一个页面 `import`，整份替换的风险是可控的。
- 备选：章号按实际渲染的章重排，让页面上永远是连续的 `01`–`06`。否决：章号是这块内容的固定编号（「03 我的位置」在任何站点上都是 03），重排会让两个用户对着同一页说「第三章」时指的不是同一块东西；缺章留空号是报表的常规做法。
- 备选：入场动效保留 `opacity` 淡入。否决：taste skill 的禁令里写明首帧必须可读；而且 v3 去掉了 `IntersectionObserver`，纯 CSS 级联下用 opacity 会让首屏之外的内容在截图与打印时整片透明。
- 备选：断轴改成对数轴。否决：对数轴要求读者会读对数刻度，而这页的读者是来看「谁用得多」的；断轴加一句明写的「轴在 <T> 压缩」比对数轴诚实，也只在确实需要时才出现。
- 备选：Geist 走 Google Fonts。否决：D16 与 D19 都记过——CSP 没放行那两个域名，`@import` 会被静默拦截（页面不报错，字体直接不生效），为一张页面放宽全站 `style-src` / `font-src` 不划算。


### D24：Cost 是第三个 Metric，金额与 tokens 同一套档位规则（2026-09-13）

**取代关系**：本条取代 D3 的「Metric 只有两项」、Non-Goals 的「不展示任何金额，也不提供按金额排序的选项」、D10 与 D14 的「响应不下发任何金额字段」，以及 v1 / v2 两段重设计背景里「金额仍然永不出现」那句。D1 的三档语义、D5 的名次规则、D8 的「请求路径只读」与 D18 的逐条裁剪方法一个字不改——改的只是「金额算不算可下发的量」。

- Metric 从两项扩成三项：`total_tokens`、`successful_requests`、`cost`。Cost = 该 Window 内 `usage_logs.actual_cost` 之和（实际计费金额，USD）。用 `actual_cost` 而不是 `total_cost`，因为用户自己的用量页「花费」用的就是它，而且成功落账口径（`usageLogSuccessFilterUL`）本来就是 `actual_cost > 0`，两个口径同源才对得上账。
- 隐私档位规则与 tokens 完全一致，不另立一套：`named` 档与 Preview 下发绝对金额（USD，`float64`），`anonymous` 档下他人只给 `cost_relative_percent`（相对该 Window 第一名的整数百分比，第一名为 `100`），本人条目与 `my_rank` 始终是真实金额。与 tokens 一样是「档位决定字段是否存在」而不是「档位把字段清零」。
- 不新增趣味卡（不做 `highlights.top_cost`）。站点合计 `highlights.site.cost` 要给（`named` 档与 Preview，`anonymous` 档缺席），04 章的解读句「前三名占全站 N%」在 `metric=cost` 下拿它做分母。
- 金额在 Snapshot 内部一律用定点 micros（1 USD = 1e6，`int64`）：ZSET 分数、Hash 第 13 段、名次计算与「还差多少进前 10」都用 micros 算，只在响应组装时用 `LeaderboardCostUSD` 换回 USD。理由是 ZSET 的 score 是 float64、名次要用 `ZCOUNT` 做严格比较，浮点金额直接进 score 会让「同一个金额」在并列判定上不稳定。
- 落到既有结构上的四处扩容：每个 Window 从两个 ZSET 变三个（key 后缀就是 metric 字符串，`metricKey` 天然支持）；Hash value 从 12 段扩到 13 段，第 13 段是 Cost micros，解码沿用 D20 的「缺段按 0」——12 段式旧值解出 `CostMicros = 0`，下一轮作业最长 5 分钟后补齐；`leaderboard_rank_history` 加第三列 `rank_cost`（迁移 241），每轮与另两列一起写；`LeaderboardMyRankHint.Value` 从 `*int64` 改成 `*float64`，并新增 kind `cost_to_top10`（常量 `LeaderboardHintCostToTop10`）——tokens / requests 两种 hint 仍是整数，JSON 形态不变，cost 的差额按 micros 算完再换回 USD。
- 名次历史表虽然攒了 `rank_cost`，页面上的「近 N 天名次」折线仍按 Total Tokens 的名次画，不给折线加 Metric 切换：那条折线是「你在总榜上的走势」，一条线一个口径最省解释成本；列先攒着，等确实需要再出。
- 备选：金额只在 `named` 档下发、`anonymous` 档整列缺席。否决：那会让匿名档的第三个 Metric 变成一列空白（排序仍按金额，却一个数都看不到），与 tokens 列在同一档下仍给相对百分比的处理自相矛盾。
- 备选：金额用 `total_cost`（账面成本）而不是 `actual_cost`。否决：用户用量页「花费」用的是 `actual_cost`，两个口径并存会让用户拿两个页面对账时对不上，这是 D3 拒绝「排除 cache 类 tokens」时用过的同一条理由。
- 备选：响应直接下发 micros，由前端换算。否决：micros 是存储层的定点技巧，不是接口口径；下发 micros 等于把这个技巧漏给每一个客户端，且前端还要再定一次「1e6」这个常量。
- 备选：给 Cost 也出一张趣味卡（`highlights.top_cost`「花得最多」）。否决：四张 Highlights 讲的是「谁最多」，卷王那张已经是同一个人的概率极高，多一张只是把同一件事说两遍；站点合计里给 `site.cost` 足够支撑解读句。


### D25：榜单头像（2026-09-14）

**取代关系**：本条只扩写身份的形态，不改任何档位语义。D10 的「身份以结构化形式下发、后端不拼展示名」与 D17 的「`identity` 仍是结构化的 `{kind, username?}`」在本条之后一律读作 `{kind, username?, avatar_url?}`——多出来的 `avatar_url` 与 `username` 受同一条规则约束（只有 `named` 形态可能有）。D1 的三档语义、D5 的 Rank / Ordinal 规则、D8 的「请求路径只读」与 D24 的金额档位一个字不改。

- 头像与展示名走同一套身份规则，不另加开关：`named` 形态才可能有 `avatar_url`；用户在个人资料里关掉「在排行榜显示我的昵称」之后，名字与头像一起消失。给头像单独做一个开关等于让「匿名但有脸」这种状态成立，而一张自拍比一个昵称更能指认到人。
- `anonymous` 档下他人的行 MUST NOT 带头像。匿名档的全部意义是「看不出是谁」，下发头像等于当场去匿名，比下发 `username` 还直接；匿名行渲染成一个不带字符的素色圆圈，占位与实名行一致，避免整列对不齐。
- 本人行始终显示自己的头像，但它 MUST NOT 由后端下发：前端从 auth store 里的 `user.avatar_url` 取（那是查看者自己的个人资料，本来就在内存里）。理由与 D10 让 `self` 的展示名由前端渲染同源——本人行只有本人看得见，没有必要让它经过一遍「把身份推给第三方看」的校验，也省掉后端为 `self` 多分一个岔路。
- 榜单 JSON 里放的是小图而不是原图：`user_avatars` 新增 `thumb_url` 列（迁移 242），存 64px 正方形 JPEG 的 data URL（约 1–3 KB），在上传 inline 头像时由 `normalizeInlineUserAvatarInput` 顺手派生（中心正方形裁切 → `xdraw.CatmullRom` 缩到 64 → 白底 → `jpeg.Encode` quality 80）；榜单只下发它。原图是 20 KB 量级，50 行乘上去就是一个兆级的响应，而页面上它只占 24px。存量的 inline 头像（线上 4 张）由进程启动时跑一次的 `UserService.BackfillAvatarThumbs` 回填，单行失败只记日志跳过，不让一张坏图卡住整轮。
- `remote_url`（外链）头像没有小图，榜单 MUST NOT 显示，前端回退首字母圆圈。外链地址是用户自填的，把它放进一个全站所有人都会打开的页面，等于让每个查看者的浏览器去请求一个任意第三方地址——那是一次由我们发起的分发，会把查看者的 IP 与 UA 交给地址的持有者，也给了填地址的人一个统计「谁看了榜单」的通道。
- 展示位置只有三处，都是名字前 24px：榜单表每行（`.rp-lb-name`）、模型偏好画像 profiles 的 50 行、Highlights 里「tokens 领先者」那张卡的 `.rp-who`（同一张卡的对比条 `.rp-cmp-t` 不加，那里是数值不是身份）。「你的位置」区块不加——它根本不渲染名字；Extremes 六张卡不加——它们的 who 是纯文本列表；报头不加。
- 数据形态：`service.UserAvatar.ThumbURL` 与 `service.UpsertUserAvatarInput.ThumbURL`（存储层，inline 必填、`remote_url` 留空）、`service.LeaderboardIdentity.AvatarURL string` 带 `json:"avatar_url,omitempty"`（下发层，只在 `kind == named` 且该用户有非空 `thumb_url` 时出现）、前端 `LeaderboardIdentity.avatar_url?: string`。榜单构建时只在 `named` 档（`renderNamed` 为真）调一次 `GetUserAvatarThumbsByUserIDs`（只取 `thumb_url` 一列：原图列每行可达 20 KB，榜单 MUST NOT 为读 2 KB 的小图搬 1 MB 原图），与 `GetByIDs` 同一批 id；`anonymous` 档一次都不查。该查询失败只记一条日志、榜单照常下发——头像是装饰，不是数据，没有理由让它把整个响应拖失败。Highlights / Extremes / profiles 的身份都经 `leaderboardHighlightIdentity` 从 `slots` 取，自然继承同一个 identity。
- 解码加固（审查后补，2026-09-14）：头像字节数上限（100 KB）挡不住「几 KB 的文件、上亿像素」的解码炸弹（1-bit 调色板 PNG、无损 webp），而小图派生让 ≤20 KB 的头像也要解码了。所有头像解码统一走 `decodeAvatarImage`：先 `image.DecodeConfig` 读文件头，单边 > 8192 或像素数 > 16M 直接按无效头像拒绝，>20 KB 的压缩路径同样受此保护；大图先用 `ApproxBiLinear` 粗缩到 256px 再 `CatmullRom` 到 64px。回填按 `user_id` 键集分页（游标越过解不开的坏图，后面的行不会被卡住），写回的 UPDATE 带 `storage_provider='inline' AND thumb_url=''`（读写之间用户换了头像就落空，不把旧图的小图盖上去）。
- 备选：榜单直接下发原图的 data URL，或给每行一个 `/api/v1/user/avatar/{id}` 之类的取图地址。否决：前者把响应撑到兆级；后者等于给一次榜单渲染追加 50 个并发请求，还要为「谁能看谁的头像」再实现一遍档位判定——而档位判定已经在响应组装层做过一次了，第二份实现迟早与第一份走散。
- 备选：给头像单独一个「在排行榜显示我的头像」开关。否决：两个开关四种组合，其中「显示头像但不显示昵称」比两个都关更糟（脸比名字更能指认人），产品上没人想要，设置页上也解释不清；一个开关同时管住名字与脸，语义才是闭合的。
- 备选：`anonymous` 档下把头像模糊或马赛克后再下发。否决：像素化之后仍然留着主色与轮廓，熟人一眼能认；而且它让页面看起来像「这里有东西被藏起来了」，比一个干净的素色圆圈更招人去猜。
- 备选：外链头像也上榜，由后端先抓回来转成小图。否决：那是让服务端按用户自填的地址发出站请求（SSRF 面），为一张 24px 的图开这个口子不划算；回退首字母圆圈在视觉上已经足够，而且榜单上本来就有一半是匿名的空圆。

## Risks / Trade-offs

- [`named` 档下关掉了昵称展示的用户仍是假名 + 精确数值，熟人可对着仪表盘数值反查身份] → 见 ADR-0002 的 Consequences：这是管理员选择最开放档时接受的残余风险，设置项旁写明；想要更严就用 `anonymous` 档。
- [`named` 档 + 今日窗口 + 5 分钟刷新，对显示着昵称的用户构成粗粒度活动时间线] → 昵称展示默认开启之后这不再是「用户自己同意的范围」，设置项旁 MUST 直接说明这条风险并指向个人资料里的关闭入口（opt-out）。
- [`anonymous` 档在小站点形同虚设：人少时相对百分比也能对上号] → 参与人数少于 5 时不展示榜单条目，只显示本人行。
- [本月窗口的聚合要扫 `usage_logs` 约三分之一的数据，规模大时可能到秒级] → 上线前用生产量级数据跑 `EXPLAIN (ANALYZE, BUFFERS)` 并记录 p99；达到秒级直接建按用户 × 天的预聚合表或覆盖索引，不留给 v2。作业在后台跑，即使慢也不阻塞任何请求。
- [Redis 不可用或 key 尚未生成时页面没有数据] → 明确的「正在计算」状态而不是空榜；作业的选主在 Redis 不可用时回落到 Postgres advisory lock，恢复后下一轮即重建。
- [Snapshot 最长可能陈旧 5 分钟，用户对着自己的仪表盘会发现对不上] → 页面常驻显示 Snapshot 更新时间；超过 15 分钟未更新时升级为警告。
- [管理员切换模式后，guard 的进程内缓存让旧模式再生效几秒] → 读取器 TTL 取 5 秒量级，最坏情况是几秒内仍按旧档渲染；不靠清空 Snapshot 解决，因为身份与精度在响应组装层决定。
- [用户被紧急禁用后仍出现在榜上] → 身份与资格在响应时按 `users` 当前状态判定，下一次响应即从条目消失；Participant Count 与 Rank 最长 5 分钟后修正。
- [`Successful Requests` 与管理端 User Breakdown 的请求数口径不同，管理员两边对账会困惑] → 差异已写进 `CONTEXT.md` 词条，页面上注明「只计成功落账的请求」。
- [DST 跳变日与 `usage_logs` 保留期短于本月窗口时本月榜少算] → 记入文档，与站点既有日聚合行为保持一致，不做特殊处理。
- [设置接入点多达十余处，漏掉任意一处会让开关静默失效] → 仓库已有 `api_contract_test.go` 的 wantJSON 与 `public_settings_injection_schema_test.go` 两道契约测试，任务中包含同步更新它们。
- [自托管 Fraunces 与 Inter 的 woff2 会增加前端产物体积] → 只取拉丁子集与实际用到的字重（Fraunces 400/500/600 与 italic 400/500，Inter 400/500/600/700）；`@font-face` 只在 `leaderboard-tokens.css` 里声明，随 `/leaderboard` 的懒加载路由按需加载，不进全局入口；两条兜底栈保证字体拿不到时页面仍然可读。
- [`usage_dashboard_daily` / `usage_dashboard_hourly` 受 `cfg.DashboardAgg.Enabled` 控制，关掉时整张表不更新] → Insights 的四个区块按「预聚合缺行即 null」处理，前端隐藏该区块；榜单本体、Highlights 与 `models_today` 都不依赖这两张表，因此关掉预聚合只会少几块洞察，不会让页面报错或显示 0。
- [aurora、卡片光晕、进度条流光与骨架 shimmer 这些持续动效在低端设备上掉帧] → 持续动效集中在 `.cr-ambient` 一个开关后面，默认值取 `!prefers-reduced-motion` 并 `localStorage` 持久化；`prefers-reduced-motion: reduce` 时样式层把时长压到 `0.01ms`、脚本层直接跳终值；入场动效只跑一次，条形生长用 `width` 过渡而不是逐帧脚本。
- [`leaderboard_rank_history` 按「人 × 天」增长，作业每 5 分钟对 `today` 窗口的全部参与者 upsert 一次，写入量与参与人数成正比] → 每天每人只有一行，覆盖写不产生新行，稳态行数是「参与人数 × 90」；一万人的站点约九十万行，主键 `(user_id, snapshot_date)` 就是唯一索引，upsert 分批 1000 行、每 5 分钟一次，对写入通道的占用可以忽略。真正的增长风险在保留期，因此清理写在同一轮作业里而不是靠人工。达到量级问题时先把 upsert 改成每日一次，再考虑按天分区。
- [`streak` 要回溯 90 天的 `usage_dashboard_daily_users` 做 gaps-and-islands，随站点存量线性变大] → 这张表每天每人只有一行，90 天的切片是「参与人数 × 90」量级，且有 `bucket_date` 索引可以先按日期裁剪；整条查询在后台作业里跑，即使慢也不阻塞任何请求。与月窗口聚合一样，上线前用生产量级数据跑一次 `EXPLAIN (ANALYZE, BUFFERS)`；达到秒级就把回溯窗口从 90 天缩短，或改成只对 Top N 用户算。
- [`named` 档下 Cost 是绝对金额，等于把每个显示着昵称的用户的消费规模对全站公开] → 这是 D24 明确接受的代价：金额与 tokens 走同一套档位规则，不接受的运营者用 `anonymous` 档（他人只剩 `cost_relative_percent`）或 `off`。风险与「`named` 档 + 今日窗口构成活动时间线」同源，退出通道仍是个人资料里的昵称展示开关。
- [Hash 从 12 段扩到 13 段、ZSET 从 2 个增到 3 个，旧快照与新代码并存] → 沿用 D20 已经验证过的「缺段按 0」：12 段式旧值解出 `CostMicros = 0`，表现为该窗口的金额列全 0 而不是解码失败，最长 5 分钟后被下一轮重建补齐；第三个 ZSET 在旧快照上不存在，读到缺失即按「正在计算」处理。回滚同理，旧代码只读前 12 段、忽略第三个 ZSET。
- [`viewer.models` 在 D8「请求路径只读」上开了一个口子，一旦被扩用就会把整条链拖回实时聚合] → 例外的边界写死在三处并各有测试：SQL 必须带 `user_id = 查看者`、只服务 `viewer.models` 这一个字段、结果在 Redis 上按 `(user_id, window, 窗口起点)` 缓存 60 秒。最坏情况是每个活跃用户每分钟一条只扫自己数据的聚合，走 `(user_id, created_at)` 索引；配合 `panelRateLimiter.Heavy()`，它的量级与「用户自己的用量页」同级。Redis 不可用时这一块降级为空数组，MUST NOT 让整个响应失败。
- [`named` 档下头像把「用了多少」与一张脸绑在一起，比 `username` 更能指认到具体的人] → 头像与昵称受同一个开关约束（D25），用户关掉「在排行榜显示我的昵称」之后两者一起消失；`anonymous` 档下他人一律没有头像，本人的头像不经后端下发。残余风险与「`named` 档下显示昵称」同源，退出通道也是同一个，不额外增加一条。
- [`thumb_url` 是 data URL，跟着榜单响应一起走，50 行就是几十到一百多 KB 的额外体积] → 小图定死在 64px / JPEG quality 80，实测单张 1–3 KB；只有 `named` 档且实名的行才带，`anonymous` 档一张都不带；`remote_url` 头像没有小图因此也不占体积。真正的上限是 50 行，与条目数同阶，不随站点规模增长。再嫌大就把 thumb 换成独立的缓存端点，但那要先付「第二份档位判定」的代价（见 D25 的否决项）。

## Migration Plan

1. 新增迁移文件，编号取 `backend/migrations` 下现有最大编号加一：当前最大为 `237_add_minimax_platform.sql`，因此本变更用 `238_user_leaderboard_named_participation.sql`。内容为 `ALTER TABLE users ADD COLUMN IF NOT EXISTS leaderboard_named_participation BOOLEAN NOT NULL DEFAULT false;`，幂等，普通事务迁移即可（新增带默认值的布尔列在 PostgreSQL 11+ 不重写表，无需 `CONCURRENTLY`，因此不使用 `_notx.sql` 后缀）。同步更新 `backend/ent/schema/user.go` 并重新生成 ent 代码。
2. 部署包含该迁移的后端版本。`leaderboard_mode` 未写入时按默认值 `off` 解析，存量站点部署后行为完全不变：接口对普通用户 404，侧边栏无入口，后台作业照常运行但只写 Redis。
3. 管理员在设置页把 Leaderboard Mode 调到 `anonymous` 或 `named` 后功能才对普通用户可见。首次开启后最长等待一个作业周期（5 分钟）Snapshot 才就绪，期间页面显示「正在计算」。
4. 上线前按 D6 的约定，用生产量级数据对月窗口聚合跑一次 `EXPLAIN (ANALYZE, BUFFERS)` 并记录 p99；若达到秒级，在本变更内追加预聚合表或覆盖索引的迁移（`CREATE INDEX CONCURRENTLY` 用 `_notx.sql` 后缀）。
5. 回滚：把 `leaderboard_mode` 改回 `off` 即可让功能对所有普通用户消失，无需回滚代码。需要回滚代码时直接回退版本，新增列保留无副作用（默认 `false`，无其他读者）；Redis 上的 `leaderboard:v1:*` key 会随 TTL 自然过期，无需清理。确需彻底清理时手工 `ALTER TABLE users DROP COLUMN leaderboard_named_participation`。
6. v2 重设计新增一张表，迁移编号取现有最大加一：当前最大为 `238_user_leaderboard_named_participation.sql`，因此用 `239_leaderboard_rank_history.sql`。内容是 `CREATE TABLE IF NOT EXISTS leaderboard_rank_history (user_id BIGINT NOT NULL, snapshot_date DATE NOT NULL, rank_total_tokens INT NOT NULL, rank_successful_requests INT NOT NULL, PRIMARY KEY (user_id, snapshot_date));` 外加一条 `CREATE INDEX IF NOT EXISTS idx_leaderboard_rank_history_snapshot_date ON leaderboard_rank_history (snapshot_date);`（保留期清理按日期删，需要这条索引）。整份幂等，是新建表因此没有锁表风险，普通事务迁移即可，不用 `_notx.sql` 后缀——与 238 同理，正文与注释里都 MUST NOT 出现并发建索引的那个关键字，迁移校验器对非 `_notx.sql` 文件是整文件裸匹配。
7. v2 的回滚：页面与响应字段随代码版本回退，新表保留无副作用（没有其它读者，也不参与任何外键）；确需彻底清理时手工 `DROP TABLE leaderboard_rank_history`。`leaderboard_mode` 改回 `off` 同样能让整个功能对普通用户消失，与 v1 一致。Redis 上 `leaderboard:v1:viewer:models:*` 这一类新 key 在旧代码上根本不存在，随 60 秒 TTL 自然消失。
8. Redis 上的旧快照与新代码兼容，不需要为这一轮单独做 Redis 迁移：Hash value 从 `"tokens,requests"` 扩成 `"tokens,requests,input,cache_read"` 后，解码按逗号切分再逐段取值，段数不足时缺的两个数一律按 0 处理，因此上一版写入的两段式 value 仍能读（表现为该窗口「无缓存命中率」而不是 0%），下一轮作业最长 5 分钟后重建即补齐。新增的 `:highlights` 与 `leaderboard:v1:insights:*` 两类 key 在旧快照上根本不存在，读到缺失即按 null 处理，页面隐藏对应区块。回滚同理：旧代码只读前两段，多出来的两段会被忽略，`:highlights` 与 `insights:*` 随 TTL 自然过期。v2 把 Hash value 再从 4 段扩到 12 段（顺序见 D20），走的是同一条规则：缺的段一律按 0，因此 2 段式与 4 段式的旧 value 都仍能读，表现为对应的之最卡缺席而不是 0；回滚时多出来的八段同样被忽略。
9. Cost 这一轮（D24）新增一个迁移，编号取现有最大加一：当前最大为 `240_leaderboard_named_participation_default_true.sql`，因此用 `241_leaderboard_rank_history_cost.sql`。内容是 `ALTER TABLE leaderboard_rank_history ADD COLUMN IF NOT EXISTS rank_cost INT NOT NULL DEFAULT 0;` 外加一条 `COMMENT ON COLUMN`，幂等，普通事务迁移即可，不用 `_notx.sql` 后缀——新增带默认值的 INT 列在 PostgreSQL 11+ 不重写表。与 238 / 239 / 240 同理，正文与注释里都 MUST NOT 出现并发建索引的那个关键字（`migrations_runner.go` 对非 `_notx.sql` 文件是整文件裸匹配）。回滚时该列保留无副作用（默认 `0`，旧代码不读它），确需彻底清理时手工 `ALTER TABLE leaderboard_rank_history DROP COLUMN rank_cost`。
10. Redis 上 D24 的兼容同样不需要单独迁移：Hash value 从 12 段扩到 13 段（第 13 段是 Cost micros），解码仍按逗号切分逐段取值、缺段按 0，因此 2 / 4 / 12 段式的历史 value 全都仍能读出榜单本体，表现为金额列为 0 而不是解码失败，下一轮作业最长 5 分钟后补齐；每个 Window 新增的第三个 ZSET（key 后缀 `cost`）在旧快照上不存在，读到缺失即按「正在计算」处理。回滚同理：旧代码只读前 12 段并忽略 `cost` 这个 ZSET，多出来的一段与一个 key 随 TTL 自然过期。
11. 头像小图这一轮（D25）新增一个迁移，编号取现有最大加一：当前最大为 `241_leaderboard_rank_history_cost.sql`，因此用 `242_user_avatars_thumb.sql`。内容是 `ALTER TABLE user_avatars ADD COLUMN IF NOT EXISTS thumb_url TEXT NOT NULL DEFAULT '';` 外加一条 `COMMENT ON COLUMN`（写明空串的含义是「没有小图」，`remote_url` 头像永远是空串），幂等，普通事务迁移即可，不用 `_notx.sql` 后缀——带常量默认值的 `ADD COLUMN` 在 PostgreSQL 11+ 不重写表。与 238 / 239 / 240 / 241 同理，正文与注释里都 MUST NOT 出现并发建索引的那个关键字（`migrations_runner.go` 对非 `_notx.sql` 文件是整文件裸匹配）。存量的 inline 头像不由迁移回填，而是在进程启动时由 `UserService.BackfillAvatarThumbs` 分批补齐（原图在列里，派生小图要解码与缩放，不是 SQL 能做的事）；回填失败的单行只记日志，下次启动再试，既不阻塞启动也不影响榜单——缺小图的行就是不显示头像。回滚时该列保留无副作用（默认空串，旧代码不读它），确需彻底清理时手工 `ALTER TABLE user_avatars DROP COLUMN thumb_url`。
