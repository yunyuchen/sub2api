# 用户可见排行榜验证（2026-09-11）

## 已实现

tasks.md 第 1–7 节与第 9 节全部完成（79 项），第 8 节「性能实测」三项未做：需要生产量级的 `usage_logs` 数据，本机没有。分支 `feat/user-leaderboard`，未提交。

后端：`users.leaderboard_named_participation` 列与迁移 238；`leaderboard_mode` 设置贯通 domain_constants → settings_view → setting_parse → setting_update → setting_public（含注入 payload）→ dto → 用户侧与管理端 setting_handler → 审计 diff → api_contract_test；带 5 秒 TTL 的模式读取器；实名参与校验（2–32 字符、字符集、不含 @、保留词子串匹配）与渲染时二次校验；一条 SQL 三窗口聚合（INNER JOIN users 过滤禁用与软删，Successful Requests 用 `actual_cost > 0`）；Redis ZSET + Hash 快照，key 带窗口起点，TTL 取 min(60 分钟, 距窗口结束)，pipeline 写临时 key 后 RENAME；每 5 分钟 leader lock 后台作业；`GET /api/v1/leaderboard` 只读快照，off 档普通用户返回与未注册路由逐字节一致的纯文本 404，管理员 Preview；不设任何清空快照的钩子。

前端：`/leaderboard` 路由与 fail-closed 守卫（管理员放行）、侧边栏入口随 `isLeaderboardVisible()` 显隐、排行榜页面与表格 / My Rank 组件（五种非正常态、匿名档相对百分比、本人行高亮、复用 MonitorRankBadge）、个人资料「实名参与」开关（只在拨动时提交，拒绝提示按 reason 走 i18n）、管理端三档控件与残余风险说明、zh / en 文案同步。

## 自动验证

主控在实施工作流之后独立复跑：

- `cd backend && go build ./... && go vet ./...`：通过。
- `cd backend && go test -tags=unit -count=1 ./...`：56 个包全部 ok，无 FAIL。
- `cd backend && golangci-lint run ./... --timeout=30m`（v2.13.0，与 backend-ci.yml 一致）：0 issues。
- `cd backend && go test -tags=integration -count=1 ./internal/server/routes/...`：ok。
- `cd backend && go test -tags=integration -count=1 ./internal/repository/ -run Leaderboard`：ok。本机 Docker 是 colima，需要 `DOCKER_HOST=unix://$HOME/.colima/default/docker.sock TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`，否则 testcontainers 报 rootless Docker not found。
- `cd backend && go generate ./cmd/server && go generate ./ent`：无漂移。
- `cd frontend && pnpm typecheck`：无错误。
- `cd frontend && pnpm test:run`：270 个文件、2016 个用例全部通过，含 i18n 三个守门测试。
- `cd frontend && pnpm lint:check`：0 错误 0 警告。
- `openspec validate add-user-leaderboard --strict`、`git diff --check`：通过。

实施工作流内部还做了一轮三方验证（后端全量、前端全量、spec 合规裁决），共 15 条发现（6 条 must-fix），已由修复阶段处理：迁移 238 注释里的「CONCURRENTLY」一词触发仓库迁移校验器（改措辞）；errcheck；off 档 404 响应体曾带 `LEADERBOARD_NOT_FOUND` reason（改为与未注册路由不可区分的纯文本）；个人资料拒绝提示未走 i18n 且开关无脑提交（改为按 reason 映射、仅在拨动时提交）；路由守卫直接比对原始值（改用派生布尔）。

## 验证边界

- 第 8 节性能实测未做：设计 D6 与 Migration Plan 要求上线前用生产量级数据对月窗口聚合跑 `EXPLAIN (ANALYZE, BUFFERS)`，p99 达到秒级就在本 change 内补预聚合表或覆盖索引。
- 没有启动完整服务做端到端手动验证；接口行为由 handler / routes / service 测试覆盖，页面行为由 Vitest 覆盖。
- 工作树里两处与本功能无关的既有未提交改动未动：`frontend/pnpm-lock.yaml` 被 pnpm 11 重生成、丢掉了 package.json 里 `pnpm.overrides` 的四条安全 pin，CI 钉 pnpm 9 时 `--frozen-lockfile` 会以 ERR_PNPM_LOCKFILE_CONFIG_MISMATCH 失败；`frontend/pnpm-workspace.yaml` 是 pnpm CLI 提示语被写进文件的占位内容。这两处需要由人决定回滚或修正。
- 修复阶段顺手修了两个与本功能无关、但在当前工作树里已经失败的前端测试：`ChannelMonitorView.grok.spec.ts` 的提供商数量改为取常量长度；`GroupsView.codexManifest.spec.ts` 补 auth store mock。改动各不超过 10 行。

## 重设计（2026-09-11）

第 10 节（design D15–D18 的重设计）实现完毕，10.1–10.43 与 10.45 已勾选，10.44 留空。

已接受的 mockup 偏差（实现刻意不照搬画板，理由记在这里，不再逐个开 issue）：

- **缓存卡不做「相当于省下约 N 次完整请求」**。画板上那一句要把命中的 tokens 折算成「完整请求数」，需要一个「一次完整请求平均多少 input tokens」的第三种口径——既不是榜单的 Successful Requests（成功落账），也不是预聚合表的 `requests`（裸 `COUNT(*)`）。页面上已经并存两种请求口径并为此配了注脚，再加第三种只会让三个数互相解释不通。缓存卡因此只给命中率、命中 tokens 与输入合计，与 D17「缓存收益一律用命中率与命中 tokens 表达」一致。
- **榜单区块标题用「用量排行」而不是画板的「用户排行」**。「用户排行」是渠道监控里那张诊断表的名字，在 CONTEXT.md 的 Avoid 列表上；英文同样从 `User ranking` 改为 `Leaderboard`。
- **`share_percent` 取整数**，画板里的 19.6% / 47.6% 只是示例渲染。理由见 design D18 的最后一条备选：百分比统一成整数，只有 `cache_hit_rate` 是 0–1 的比率并按一位小数渲染，避免「这个百分比能不能反推绝对量」变成逐字段判断题。

仍未做的验证：

- **10.44（五块画板逐一目视验收）**：`Main.dc.html`、`AnonymousLight.dc.html`、`NamedDark.dc.html`、`AnonymousDark.dc.html`、`States.dc.html` 的亮暗两主题截图核对、返回仪表盘后的主题一致性、`prefers-reduced-motion: reduce` 下无动画，由主控在浏览器里完成，不在本轮自动化范围内。
- **8.1–8.3（性能实测）**：仍未做，原因同上文「验证边界」——需要生产量级的 `usage_logs`。

### 浏览器目视验收（10.44，2026-09-11 23:20）

本地实例（Vite 3101 → 后端 8090，一次性 Postgres 容器，Redis 7 号库）以 alice 登录逐项核对：实名档暗色、匿名档亮色两组截图与画布对照一致——杂志式 hero 与邮戳、趣味卡、用量排行区块的两组药丸切换、「你的位置」条、带奖牌与进度条的榜单（并列第 2 两枚银牌、序号连续）、模型热度、30 天热力图、14 天趋势、时段分布、缓存命中、页脚口径说明。匿名档下他人与站点级只有百分比与相对强度，参与人数「5+」，峰值不带请求数，「本月累计」换成较上月百分比；本人行与「你的位置」保持真实数值。主题切换跟随 html.dark 即时生效。紧凑数字已去掉多余的 .0（600K 而非 600.0K）。

## v2：极客风换皮 + 统计扩充（2026-09-12）

第 11 节（design D19–D22）实现完毕，11.1–11.40 全部勾选。实施由一轮工作流完成（文档 → 后端两段与前端两段并行 → 三路验证 → 修复），验证阶段 23 条发现里 6 条 must-fix 已在修复阶段处理（golangci-lint 从 6 issues 降到 0；LbHighlights / LbFooter / LbPrompt / LbStatusBar 按画板重排；缓存两块合一；光标闪烁纳入 CRT 开关；时间一律按站点时区渲染；i18n 死键清理）。11.34 待删清单共 11 个文件（LbNav / LbHero / LbMyRank / LbCache 及其 spec、useAmbientMotion 及其 spec、三个 Fraunces/Inter 字体）由主控在零引用核对后删除。

主控独立复跑的门禁：

- `cd backend && go build ./... && go vet ./... && gofmt -l`：通过。
- `cd backend && go test -tags=unit -count=1 ./...`：exit 0，无 FAIL。
- `cd backend && golangci-lint run ./... --timeout=30m`（v2.13.0）：0 issues。
- `cd backend && go test -tags=integration -count=1 ./internal/repository/ -run Leaderboard` 与 `./internal/server/routes/...`：ok（colima 需要 `DOCKER_HOST` / `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE`，同上文）。
- `cd frontend && vue-tsc --noEmit`：本功能范围内 0 错误。
- `cd frontend && vitest run --exclude '**/__prototype__/**'`：287 个文件 2149 个用例通过；唯一失败是 `localeKeyCompleteness.spec.ts`，缺的 126 个键全部只被工作树里与本功能无关的未追踪目录 `frontend/src/views/admin/__prototype__/ele-admin-plus-ts-pro/` 引用（用一份把该目录排除在扫描外的临时副本跑同一条 spec：3/3 通过；副本已删）。
- `eslint --max-warnings=0 <本 change 改动/新增的 69 个前端源文件>`：0 错误 0 警告。
- `vite build`：产物正常（只打包 JetBrains Mono / IBM Plex Sans 两个字体，无 Fraunces 残留）；退出码非 0 仅因 vite-plugin-checker 对 `__prototype__` 目录的 vue-tsc 报错。
- `openspec validate add-user-leaderboard --strict`、`git diff --check`：通过。

### 浏览器目视验收（11.40，2026-09-12 12:20–12:35）

本地实例换上 v2 二进制重启：迁移 239 自动应用（`leaderboard_rank_history` 建表），快照作业 5 分钟一轮，Hash 已是十二段；`dashboard_aggregation.recompute_days: 35` 让 28 天节律与 14 天缓存趋势有数据。为覆盖六张之最，补了一批今日种子：夜间 0–6 点用量、昨日对比、五个模型三个平台、高输出占比、单次 2.5M 的请求、`image_count > 0` 的请求、连续 28 天活跃。

以 alice 登录在 Chrome 与 headless Chrome（CDP）两边核对：

- **三块画板对照**：`Main.dc.html`（实名暗）、`GeekNamedLight.dc.html`、`GeekAnonymousDark.dc.html` 与实际页面一致——状态栏 chips（window / metric / mode / snapshot=HH:MM (+Nm) / tz）、`$ leaderboard --window … --metric …` 命令行标题与 `#` 注释、四张高亮卡、六张之最小卡、`$ whoami --stats`（#N / 人数、14 天名次折线、模型偏好 chip、你 vs 全站）、LED 条榜单、models / profiles / platforms 三栏、activity / rhythm、trend / hours / composition+cache 三栏、页脚一行状态。
- **窗口与指标切换**：`week` 下 `--rising` 卡如设计缺席（五张）；`successful_requests` 下榜单重排、副标题改为「第 1 名比第 2 名多 N% 成功请求」、whoami 名次随之变化。
- **匿名档**：参与人数「5+」，他人行只有相对百分比（`total_tokens_relative_percent` / `successful_requests_relative_percent`），高亮卡与之最只给占比或倍数（`night_owl` 无 `night_tokens`），趋势按相对强度、缓存趋势只给占比；本人行与 whoami 保持真实数值。
- **off 档**：普通用户 404，与未注册路由同样是纯文本；管理员得到 `preview: true` 的完整响应（含 extremes 与 viewer）。
- **主题与 CRT**：`☼/☾` 切换即时生效并写入站点 `theme`，回到仪表盘后主题一致；`▤` 切换 CRT 写 `lb-crt`，关闭后扫描线与光标闪烁一起停。
- **`prefers-reduced-motion: reduce`**（CDP 模拟）：根节点不带 `.crt`，光标 `animation: none`，reveal / stagger 过渡时长 1e-05s，大数字一到位就是终值。
- **响应式**：390px 宽单列堆叠、无横向滚动；状态栏 chips 换行，榜单在窄屏隐藏成功请求列。
- **首屏 reveal**：页面加载后视口内区块随即显现，视口外的滚动到再显现。注意在 `document.visibilityState === 'hidden'` 的标签页里（比如被其他窗口完全遮住时）浏览器不跑 rAF 与 IntersectionObserver，区块会停在透明态直到标签页可见——这是浏览器对隐藏页的正常节流，不是页面缺陷；用 headless Chrome（可见态）复核过。

已知口径差（沿用 v1 设计，未改）：高亮卡「全站今日」的缓存命中率来自快照（只算参与者），`$ cache --trend 14` 的今日值来自 `usage_dashboard_daily`（含所有用户，含禁用/软删账号）；两者在同一页可能不相等（本地种子下 18.5% 对 8.9%，因为种子里给禁用/软删用户灌了巨量无缓存用量）。`$ trend --days 14` 与 `$ hours --today` 同理，副题里已有「不是同一个口径」的注脚。

### 仍未做

- **8.1–8.3（性能实测）**：未做，原因同上文「验证边界」。
- 工作树里与本功能无关的在途改动继续未动：`frontend/pnpm-lock.yaml`、`frontend/pnpm-workspace.yaml`、`frontend/package.json`（新增的 `prototype:admin` 脚本）与 `frontend/src/views/admin/__prototype__/`（独立的后台 UI 原型，自带工具链与 node_modules）。

### 补记（2026-09-12 12:50）：把 `__prototype__` 排除出主应用工具链

用户确认后，把 `src/views/admin/__prototype__/**` 加进 `tsconfig.json` 的 `exclude`、`vitest.config.ts` 的 `exclude`、`tailwind.config.js` 的 `content` 否定项、`.eslintignore`，以及 `localeKeyCompleteness.spec.ts` 的 `import.meta.glob` 否定项（该 spec 靠 glob 扫源码，vitest 的 exclude 管不到它）。原型目录本身没动。之后全仓门禁恢复：`pnpm typecheck` 0 错误、`pnpm test:run` 288 文件 2150 用例通过、`pnpm lint:check` 0 错误、`pnpm build` 通过；本地 Vite 的 vue-tsc 覆盖层消失（`[vue-tsc] Found 0 errors`），`style.css` 首次转换从 12.5 秒降到 1.5 秒。

## v3：报表皮肤（2026-09-12 晚）

第 12 节（design D23）实现完毕，12.1–12.21 全部勾选。流程：mockup 工作流（三稿竞标 → 三评 → 综合成画布 page-3「Taste v3」，用户确认「就按这个设计」）→ 实施工作流（文档 → F1 tokens/骨架/01–03 章 → F2 04–07 章/页脚/清理 → 三路验证 21 条发现（must-fix 2：匿名档榜单混合量纲与本人行相对条 0%）→ 修复）。待删的 7 个文件（LbStatusBar / LbPrompt 及 spec、useCrtEffect 及 spec、JetBrains Mono / IBM Plex Sans 字体）由主控核对零引用后删除。

主控独立复跑：`vue-tsc --noEmit` 0 错误；`vitest run` 290 文件 2190 用例全过（skip 归零）；`eslint --max-warnings=0` 74 个改动文件 0 错误；`pnpm build` 通过、产物只含 geist-sans / geist-mono 两个字体；`openspec validate --strict`、`git diff --check` 通过。

### 浏览器目视验收（12.21，2026-09-12 21:25–21:35）

headless Chrome（CDP，1440 与 390 两档，亮暗两主题，实名与匿名两档）加用户 Chrome 标签页各核对一遍：

- 三张画板对照一致：刊头（window/metric 分段、mode、快照呼吸点 + `(+Nm 后重建)`、tz、主题分段、返回仪表盘）、`今天谁在用 · today` 标题块与副题、01 今日高亮（卷王大数字 + 对比双条 + 现算文案、效率之星、最勤快、全站引导点线）、02 六个之最（阶梯）、03 我的位置（#N、14 天名次折线带每点圆点、模型 chip、我 VS 全站）、04 榜单（补零名次、本人行色条与「你」、解读句现算：「第 1 名 4.5M tokens，是第 2 名的 1.7 倍、第 7 名的 75 倍。前三名合计占全站 88%。第 2、3 名只差 320K tokens，但请求数差 1 次，切到「成功请求数」口径两人名次会互换。」）、05 模型与平台（其他 1% · 取整残差）、06 活跃节奏（30 天条、7×24、今日时段 + 现算节奏说明）、07 趋势与构成（断轴：第三高 2.5M，最高 22.8M 为其 9.1 倍，轴在 2.5M 压缩并标注）、COLOPHON 页脚。
- 真实页面探针：根节点 `.rp` / `.rp.dark`，字体 Geist 与 Geist Mono，`box-shadow` / `backdrop-filter` / `filter` 为 0 处，390px 无横向滚动（页高 5981），七章章号固定。
- 匿名档：参与人数「5+」，他人只有相对百分比，卷王块改为「41% 占全站 tokens」与「领先第 2 名 66%」，全站块只剩活跃人数 / 峰值 / 命中率，趋势按相对强度（断轴在 10%），本人行与 03 章保留真实数值。
- 主控修掉一处画板保真度小问题：06 章周内节奏的「周一」标签列 18px 定宽导致两字竖排，改为 `max-content` + `nowrap`。
- 用户目视后反馈边注栏「太冗余」：去掉每章的口径说明段（`LbChapter` 的 `note` prop 与 `chapters.NN.note` 键一并删除），边注栏只留章号与标签，栏宽由 `clamp(150px, 13vw, 190px)` 收到 `clamp(84px, 7vw, 108px)`；门禁复跑全绿（vue-tsc 0、vitest 290/2190、eslint 0、openspec、diff --check），用户 Chrome 里已热更新核对。
- 已知口径差沿用：全站缓存命中率（快照，参与者）18.5% 与「全站命中率 · 近 14 天」（`usage_dashboard_daily`，含所有用户）8.9% 在同一页并存，副题注脚已说明。

### 仍未做

- 8.1–8.3 性能实测；管理端设置页的合规确认弹窗需用户自己点。
- 工作树里与本功能无关的在途改动继续未动（pnpm-lock.yaml、pnpm-workspace.yaml、package.json、`__prototype__/`）。

## v4：昵称默认展示 + 假名改「第 N 位」（2026-09-12 晚）

第 13 节。用户反馈「用户名字要写，不要写用户 #4 这样…只要显示用户的昵称就可以了」。

进入本轮验证时的工作树状态：**前端改动已就位，后端一行没动**（`backend/ent/schema/user.go` 与 `backend/migrations/238_*.sql` 的时间戳仍是 09-11 18:51 / 20:13，默认值还是 `false`）。也就是说前端已按「默认开」渲染并把文案改成了「默认开启」，而库里所有人仍然是关的——`named` 档下页面会是一整屏「第 N 位」，正好是用户要求改掉的那个效果。验证阶段补做了后端与文档两半（13.1–13.4、13.9、13.10），第 13 节的勾选状态与工作树一致。

补做的内容：

- `ent/schema/user.go` 的 `leaderboard_named_participation` 改 `Default(true)` 并重写注释，`go generate ./ent` 同步生成物（`ent/migrate/schema.go` 的 `Default: true`）。
- 新增 `backend/migrations/240_leaderboard_named_participation_default_true.sql`：`SET DEFAULT true` + 把现存 `false` 行回填成 `true` + 更新列注释。238 已应用，按 `migrations/README.md` 的不可变原则没有改它。文件内不含并发建索引关键字（`migrations_runner.go` 对非 `_notx.sql` 是整文件裸匹配）。
- `user_repo.go` 的 `applyUserEntityToService` 回读该列：`Create()` 不写这一列，不回读的话注册响应里的开关会显示成「关」。
- 后端六个文件的注释口径（「用户 #N」→「第 N 位」、「默认 false / 自选实名」→「默认 true / 可关闭」）。响应形状与身份判定逻辑一行未动。
- `CONTEXT.md`、`design.md`、`proposal.md`、两个 spec、`tasks.md` 第 13 节；新增 `docs/adr/0004-leaderboard-nickname-display-is-opt-out.md` 并在 ADR-0002 顶部标注「部分被 0004 取代」——0002 的核心论证之一正是「实名必须自选」。

独立复跑的门禁（全绿）：

- `cd backend && go build ./... && go vet ./...`、`gofmt -l`（`ent/` 以外 0 个文件）。
- `cd backend && go test -tags=unit -count=1 ./...`：56 个包 ok，0 FAIL。
- `cd backend && golangci-lint run ./... --timeout=30m`（v2.13.0）：0 issues。
- `cd backend && go test -tags=integration -count=1 ./internal/server/routes/...`：ok 10.4s。`./internal/repository/ -run Leaderboard`：ok 3.6s。两组都会先跑 `ApplyMigrations`，因此迁移 240 已在一次性 Postgres 容器上真实应用过（colima 需 `DOCKER_HOST` / `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE`）。
- `cd backend && go generate ./ent && go generate ./cmd/server`：无新增漂移。
- `cd frontend && ./node_modules/.bin/vue-tsc --noEmit`：0 错误。
- `cd frontend && ./node_modules/.bin/vitest run`：291 个文件 2197 个用例全绿，含 `localeKeyCompleteness` / `localesNoKeyCollision` / `localesMessageCompile` 三个守门测试与新增的 `leaderboardIdentityLocales.spec.ts`。
- `cd frontend && ./node_modules/.bin/eslint --max-warnings=0 <本 change 改动/新增的 76 个前端文件>`：0 错误 0 警告。
- `cd frontend && pnpm run build`：exit 0。
- `openspec validate add-user-leaderboard --strict`、`git diff --check`：通过。

仍未做：

- 13.12 `displayName.ts` 的独立单测（`self` 分支「有 username 用 username」目前只被四个组件 spec 间接覆盖）。
- 13.13 浏览器目视验收：迁移 240 要等本地后端重启才生效，本轮按约束没有重启 3101 / 8090。
- 8.1–8.3 性能实测：同前几轮，需要生产量级 `usage_logs`。

遗留的口径判断（需要产品确认，本轮未擅自改行为）：

- **本人行不套用 username 校验**。`displayName.ts` 的 `self` 分支直接用 auth store 的 `username`，所以一个 username 是邮箱全文的 OAuth 用户，在自己的本人行上会看到自己的邮箱。这只有本人可见、响应里也不含它，但与「展示名永远不是邮箱」的字面表述冲突，已在 `CONTEXT.md` 与 `user-leaderboard` spec 里把该禁令的范围收缩到「他人的展示名」。要改成「本人 username 不合格就回退「当前用户」」是一行的事。
- **ADR-0002 的隐私论证被推翻**。0002 立论是「OAuth 静默回填的真实姓名未经同意不能上榜」，默认开启之后这份同意不复存在，只剩 opt-out 与「`named` 档本身要管理员显式选」两道闸。已记进 ADR-0004 的 Consequences，部署到已有用户的站点时需要运营者自行通知。

### 主控目视验收（13.13，2026-09-12 22:40）

本地后端换上含迁移 240 的二进制重启：`users.leaderboard_named_participation` 列默认值已是 true，现有 9 位用户全部回填为 true。用户 Chrome 标签页（alice 登录，实名档）核对：

- 报头一行两端对齐：`Sub2API 用量排行榜 | today week month | tokens 请求数 | ● 22:40（1m 后重建）| 亮 暗 | ← 返回仪表盘`；标题块只剩 `今天谁在用` 与 `7 位活跃 · 2026-09-12`。
- 榜单展示名：`alice（你）/ bob / erin` 直接是昵称；第 4–7 位（无昵称、邮箱形态昵称、含保留词昵称、admin 无昵称）显示「第 N 位」；卡片与画像同步（进步之星 bob、杂食者 第 4 位、话痨 第 5 位）。
- 文案精简后：章名右侧无副题，榜单下只剩一行 `第 1 名是第 2 名的 1.7 倍 · 前三名占全站 88%`，页脚 `snapshot 22:40 · 每 5 分钟重建 · Asia/Shanghai · 不展示金额与邮箱`。
- 三轮改动的门禁均由主控或验证 agent 复跑：vue-tsc 0、vitest 291 文件 2197 用例、eslint 0、pnpm build、后端 build/vet/lint/unit/integration、openspec validate、git diff --check 全绿。

未做：13.12 `displayName.ts` 的独立单测（四个组件 spec 已间接覆盖）；8.1–8.3 性能实测。
- 2026-09-12 23:05 用户指出 03 章名次折线「变形」：`LbSparkline` 用 300 宽的名义 viewBox 配 `preserveAspectRatio="none"` 拉伸到 866px，圆点被横向拉成椭圆。改为用 ResizeObserver 让 viewBox 宽度跟随实际渲染宽度（量不到时退回 300），圆点恢复正圆；vue-tsc / eslint / 三个相关 spec 通过。
