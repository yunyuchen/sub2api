## Purpose

定义 Leaderboard Mode（排行榜模式）三档暴露程度的语义：谁能访问 Leaderboard（排行榜）、看到什么身份形态、看到什么精度的数值，以及 Named Participation（昵称展示）这个用户开关的校验与回退规则。本能力规定的是「暴露多少」，榜单本体的算法与页面行为见 `user-leaderboard`。

## ADDED Requirements

### Requirement: Leaderboard Mode 是三档递增的系统设置，默认 off
系统 SHALL 新增系统设置 `leaderboard_mode`，取值 MUST 只有 `off`、`anonymous`、`named` 三档，默认值 MUST 是 `off`。`off`：Leaderboard 对普通用户既不可见也不可访问，管理员可 Preview（预览）。`anonymous`：所有人以匿名形态出现，他人的数值只给相对第一名的整数百分比，Participant Count（参与人数）只给分档。`named`：开启了 Named Participation 且 `username` 合格的用户以实名形态出现，其余仍是匿名形态，所有数值与 Participant Count 精确。模式开启后管理员与普通用户 MUST 看到完全相同的数据，Leaderboard MUST NOT 做角色分支；需要邮箱、金额与任意起止日期的管理员 MUST 走 User Breakdown（用户用量明细）。

#### Scenario: 存量站点升级后行为不变
- **WHEN** 站点升级到包含本变更的版本，且从未写入过 `leaderboard_mode`
- **THEN** 系统 MUST 按 `off` 处理
- **THEN** 普通用户 MUST 感知不到任何新增功能

#### Scenario: 开启后管理员与普通用户同视图
- **WHEN** Leaderboard Mode 为 `named`，一名管理员与一名普通用户请求同一个 Window（榜单窗口）与 Metric（排名指标）
- **THEN** 两人收到的 `entries` 内容 MUST 完全相同
- **THEN** 管理员的响应 MUST NOT 包含邮箱、金额或 `user_id`

#### Scenario: 档位递增
- **WHEN** 管理员把模式从 `anonymous` 调整到 `named`
- **THEN** 他人条目的数值 MUST 从相对百分比变为精确值
- **THEN** `participant_count` MUST 从分档变为精确值

### Requirement: 读取失败按 off 处理
Leaderboard Mode 的读取 MUST fail-closed：设置读取失败、值缺失或值非法时，系统 MUST 按 `off` 处理，MUST NOT 回落到任何更开放的档位。承载 guard 的读取器 MUST 是带短 TTL 的进程内缓存实现（`atomic.Value` 快照 + `singleflight` 收敛回源），读不到或出错时 MUST 缓存 `off` 并缩短 TTL；管理员切换模式后存在秒级延迟是可接受的。guard MUST NOT 为每个请求裸查一次 settings 表。

#### Scenario: 设置读取出错
- **WHEN** guard 读取 `leaderboard_mode` 时数据库返回错误
- **THEN** 系统 MUST 按 `off` 处理，普通用户请求 MUST 返回 404
- **THEN** 系统 MUST NOT 按上一次读到的更开放档位继续服务

#### Scenario: 读取器命中缓存
- **WHEN** 大量用户在短时间内连续请求 Leaderboard
- **THEN** guard MUST 在 TTL 内复用进程内快照
- **THEN** 每个请求 MUST NOT 各自查询一次 settings 表

#### Scenario: 切换模式后的延迟
- **WHEN** 管理员把模式从 `named` 改为 `off`
- **THEN** guard 最多在一个短 TTL 之后 MUST 按 `off` 生效
- **THEN** 这段时间内 MUST NOT 出现比切换前更开放的数据

### Requirement: 写入侧白名单校验，读取侧再归一化
管理端写入 `leaderboard_mode` 时 MUST 做严格白名单校验，取值不是 `off` / `anonymous` / `named` 时 MUST 返回 400 且 MUST NOT 落库。读取侧 MUST 再归一化一次：库里出现非法值或空值时 MUST 归一化为 `off`。管理端更新接口对该字段 MUST 使用指针语义，请求未提交该字段时 MUST NOT 把已有值刷掉。

#### Scenario: 非法 mode 写入
- **WHEN** 管理员提交 `leaderboard_mode: "public"`
- **THEN** 系统 MUST 返回 400
- **THEN** 设置 MUST 保持原值不变

#### Scenario: 库里存量非法值
- **WHEN** settings 表里 `leaderboard_mode` 的值是空字符串或历史遗留的非法值
- **THEN** 读取侧 MUST 归一化为 `off`
- **THEN** 系统 MUST NOT 因为无法解析而返回 500

#### Scenario: 未提交该字段的更新
- **WHEN** 管理员提交一次不含 `leaderboard_mode` 的设置更新
- **THEN** 现有的 `leaderboard_mode` MUST 保持不变

### Requirement: off 档下普通用户 404，管理员可 Preview
Leaderboard Mode 为 `off` 时，普通用户请求 `GET /api/v1/leaderboard` MUST 返回 404，MUST NOT 返回 403——403 会确认功能存在。管理员在 `off` 下 MUST 被放行，响应 MUST 带 `preview` 标记为 `true`，页面 MUST 显示「预览，普通用户不可见」横幅。模式不为 `off` 时 `preview` MUST 为 `false`。隐私控制 MUST 在服务端强制，前端隐藏只是第二层。Preview 下响应的 `mode` MUST 回显 `off`、`preview` MUST 为 `true`，`entries` MUST 按 `named` 档的身份形态与数值精度规则渲染、`participant_count` MUST 是精确整数——管理员预览的正是开启后最开放的形态，否则仍要靠「先开后关」才能看到效果，而那恰是 Preview 要消除的短暂真实泄露。

#### Scenario: 管理员在 off 下访问
- **WHEN** Leaderboard Mode 为 `off`，管理员请求 Leaderboard
- **THEN** 系统 MUST 返回 200，`mode` MUST 是 `off`，`preview` MUST 为 `true`
- **THEN** `entries` MUST 按 `named` 档规则渲染（Named Participation（昵称展示）为开且 `username` 合格者实名，数值与 `participant_count` 精确）
- **THEN** 页面 MUST 显示「普通用户不可见」的预览横幅

#### Scenario: 普通用户在 off 下访问
- **WHEN** Leaderboard Mode 为 `off`，普通用户绕过前端直接请求接口
- **THEN** 系统 MUST 返回 404
- **THEN** 响应 MUST NOT 透露该功能存在或已被关闭

#### Scenario: 模式开启后的 preview 标记
- **WHEN** Leaderboard Mode 为 `anonymous` 或 `named`
- **THEN** 管理员与普通用户的响应里 `preview` MUST 都是 `false`
- **THEN** 页面 MUST NOT 显示预览横幅

### Requirement: anonymous 档所有人匿名，他人只给相对百分比
Leaderboard Mode 为 `anonymous` 时，所有 Leaderboard Entry（榜单条目）MUST 是匿名形态：查看者本人的 `identity.kind` 为 `self`，其余为 `anonymous`，MUST NOT 下发任何 `username`。他人条目的两个绝对数值字段 MUST 缺席，各自由 `total_tokens_relative_percent` 与 `successful_requests_relative_percent` 代替：两者 MUST 是 0 到 100 的整数，表示相对该 Metric 第一名的百分比，该 Metric 第一名的值 MUST 是 `100`——档位决定字段是否存在，而不是把字段清零，客户端 MUST NOT 有机会把缺席误读成 0。查看者本人的条目与 `my_rank` MUST 始终是真实的绝对数值。`participant_count` MUST 只给分档字符串，MUST NOT 给精确值；分档规则 MUST 是：取下界序列 5、10、20、50、100、200、500、1000、2000、5000、10000 中不超过实际人数的最大值，表达为「N+」（如 137 → 「100+」），少于 5 时为「<5」。

#### Scenario: 他人条目没有绝对数值
- **WHEN** 查看者在 `anonymous` 档下查看榜单
- **THEN** 他人条目 MUST NOT 包含 `total_tokens` 与 `successful_requests` 字段
- **THEN** 该条目 MUST 分别给出 `total_tokens_relative_percent` 与 `successful_requests_relative_percent`，两者 MUST 是 0 到 100 的整数，该 Metric 第一名的值 MUST 是 `100`

#### Scenario: 本人行仍是真实数值
- **WHEN** 查看者本人进入前 50
- **THEN** 其条目 MUST 给出真实的 Total Tokens（总 tokens）与 Successful Requests（成功请求数）
- **THEN** `my_rank` MUST 同样是真实数值

#### Scenario: 参与人数只给分档
- **WHEN** 该 Window 内的 Participant Count 实际为 137
- **THEN** `participant_count` MUST 以分档形式表达（如「100+」）
- **THEN** 响应 MUST NOT 出现精确值 137

#### Scenario: 开了实名但模式是 anonymous
- **WHEN** 某用户已开启 Named Participation 且 `username` 合格，而 Leaderboard Mode 是 `anonymous`
- **THEN** 该用户的条目 MUST 仍是匿名形态
- **THEN** 响应 MUST NOT 包含其 `username`

### Requirement: anonymous 档参与人数少于 5 时只显示本人行
Leaderboard Mode 为 `anonymous` 且该 Window 的 Participant Count 少于 5 时，系统 MUST NOT 下发任何 Leaderboard Entry，只返回查看者自己的 My Rank（我的名次）相关信息；页面 MUST 只显示本人行并说明人数太少暂不展示榜单。理由是人数很少时「假名 + 相对百分比」也接近可辨认。该门槛值是设计阶段的假设，实现时可调整，但 MUST 在 `anonymous` 档生效、MUST NOT 在 `named` 档套用。被抑制时响应 MUST 带明确标记 `entries_suppressed: true`，页面 MUST 据此把抑制态与「窗口内无人有用量」的空态区分开（两者的 `entries` 都是空数组，见 `user-leaderboard`）。

#### Scenario: 参与人数少于 5
- **WHEN** `anonymous` 档下该 Window 只有 4 个有用量的合格用户
- **THEN** `entries` MUST 是空数组且 `entries_suppressed` MUST 为 `true`
- **THEN** 页面 MUST 只显示本人行与说明文案，MUST NOT 显示其他任何条目

#### Scenario: 抑制态与空态可区分
- **WHEN** `anonymous` 档下该 Window 有 4 个有用量的合格用户，查看者在其中
- **THEN** `entries_suppressed` MUST 为 `true`，前端 MUST 据此判定为抑制态
- **THEN** 页面 MUST 显示「参与人数过少，暂不展示榜单」而 MUST NOT 显示空态文案

#### Scenario: 参与人数恰好 5
- **WHEN** `anonymous` 档下该 Window 恰好有 5 个有用量的合格用户
- **THEN** `entries` MUST 正常返回这 5 条（仍是匿名形态与相对百分比）
- **THEN** `entries_suppressed` MUST 为 `false`

#### Scenario: named 档不套用该门槛
- **WHEN** `named` 档下该 Window 只有 3 个有用量的合格用户
- **THEN** `entries` MUST 正常返回这 3 条
- **THEN** 系统 MUST NOT 因为人数少而隐藏榜单

### Requirement: named 档只对开着昵称展示且 username 合格者实名
Leaderboard Mode 为 `named` 时，某个条目以实名形态展示的充分必要条件 MUST 是：该用户的 Named Participation 为开（默认即开），且其 `username` 通过校验。不满足任一条件的用户 MUST 仍以匿名形态出现，但其数值 MUST 与其他人一样精确。`named` 档下 `participant_count` MUST 是精确值。

#### Scenario: 昵称展示为开且 username 合格
- **WHEN** 某用户的 Named Participation 为开且 `username` 合格
- **THEN** 其条目的 `identity.kind` MUST 为 `named`，`identity.username` MUST 是该 `username`

#### Scenario: 用户自己关掉了昵称展示
- **WHEN** 某用户把 Named Participation 关掉了
- **THEN** 其条目 MUST 是匿名形态（Display Name（展示名）为「第 Ordinal 位」）
- **THEN** 其 Total Tokens 与 Successful Requests MUST 仍是精确值

#### Scenario: 参与人数精确
- **WHEN** `named` 档下该 Window 的 Participant Count 实际为 137
- **THEN** `participant_count` MUST 返回 137

### Requirement: Named Participation 默认开启，开启时校验 username
系统 SHALL 在 `users` 表新增布尔列表示 Named Participation，默认 MUST 是 `true`，并 MUST 把该列建出来时已存在的用户一并回填成 `true`（迁移 240），使「默认显示昵称」对新老用户一致。个人资料页 MUST 提供一行开关，文案是「在排行榜显示我的昵称」，MUST NOT 再叫「实名参与」。开启该开关时 MUST 校验 `username`：去首尾空白后 2–32 个字符；只允许 Unicode 字母、数字、`_`、`-`、`.` 以及词间的单个空格；不得含 `@`（邮箱形态）；不得命中保留词（大小写不敏感的子串匹配：`admin`、`root`、`system`、`official`、`support`、`sub2api`、「官方」、「管理员」、「客服」、「系统」）。规则与清单作为常量维护，实现时可调。校验不通过时 MUST 拒绝开启并返回 400，MUST NOT 落库。该校验 MUST NOT 改动个人资料现有的 `username` 通用校验，以免波及注册与 OAuth 回填路径。关闭该开关的用户 MUST 仍然参与排名，它 MUST NOT 被实现成「退出排行」开关。

#### Scenario: 默认开启
- **WHEN** 一个新用户注册或存量用户升级后第一次读取个人资料
- **THEN** `leaderboard_named_participation` MUST 是 `true`
- **THEN** 该用户在 `named` 档下 MUST 是实名形态，除非其 `username` 不合格

#### Scenario: username 是邮箱形态
- **WHEN** 某用户的 `username` 是 `someone@example.com`，他尝试开启 Named Participation
- **THEN** 系统 MUST 返回 400 并拒绝开启
- **THEN** 该用户的 `username` 本身 MUST NOT 被修改

#### Scenario: username 命中保留词
- **WHEN** 某用户的 `username` 是 `admin` 或包含「官方」这类保留词，他尝试开启开关
- **THEN** 系统 MUST 返回 400 并拒绝开启

#### Scenario: 合格的 username 开启成功
- **WHEN** 某用户的 `username` 非空、不是邮箱形态、长度字符集合规且不命中保留词
- **THEN** 系统 MUST 开启该开关并在个人资料响应里回显 `true`
- **THEN** 在 `named` 档下该用户的条目 MUST 变为实名形态

#### Scenario: 关闭开关仍然参与排名
- **WHEN** 某用户关闭 Named Participation
- **THEN** 其用量 MUST 仍然计入榜单与 `participant_count`
- **THEN** 其条目 MUST 只是退回匿名形态

### Requirement: 渲染时二次校验 username，不合格回退匿名
即使某用户的 Named Participation 已经开启，系统 MUST 在渲染每个响应时对其 `username` 再做一次同样的校验；不合格时 MUST 回退为匿名形态，MUST NOT 下发该 `username`，也 MUST NOT 因此报错或把该用户整行丢弃。用户 id MUST NOT 出现在任何 Display Name 里，「User #id」这类兜底 MUST NOT 使用。

#### Scenario: 开启后把 username 改成不合格的值
- **WHEN** 某用户先开启了 Named Participation，之后把 `username` 改成了邮箱形态
- **THEN** `named` 档下其条目 MUST 回退为匿名形态
- **THEN** 响应 MUST NOT 包含该 `username`

#### Scenario: username 被清空
- **WHEN** 某已开启实名的用户的 `username` 变为空
- **THEN** 其条目 MUST 以匿名形态渲染
- **THEN** 系统 MUST NOT 用用户 id 拼出展示名作为兜底

#### Scenario: username 仍然合格
- **WHEN** 某已开启实名的用户的 `username` 通过二次校验
- **THEN** 其条目 MUST 以实名形态展示

### Requirement: 响应永不下发 user_id、邮箱与金额
Leaderboard 的任何响应，在任何档位、对任何角色，MUST NOT 包含 `user_id`、邮箱或任何金额字段。身份 MUST 只以结构化的 `identity{kind, username?}` 表达。

#### Scenario: named 档下的响应体
- **WHEN** `named` 档下管理员请求 Leaderboard
- **THEN** 响应体 MUST NOT 出现 `user_id`、`email` 或任何金额字段
- **THEN** 条目里唯一可能出现的身份信息 MUST 只有 `username`

#### Scenario: 匿名形态的条目
- **WHEN** 某条目以匿名形态展示
- **THEN** 该条目 MUST 既不含 `username`，也不含 `user_id` 或邮箱

### Requirement: 公开设置暴露 leaderboard_mode 并派生前端布尔 flag
`GET /api/v1/settings/public` 与注入到页面的公开设置负载 MUST 包含 `leaderboard_mode`。前端 MUST NOT 把该枚举直接登记进 `featureFlags.ts` 的注册表（注册表只认布尔值，枚举会恒为 `false`）；MUST 新增枚举读取器 `getLeaderboardMode()`，再由它派生一个「mode 不为 `off`」的布尔函数供侧边栏与路由守卫使用。公开设置读不到该键时，前端 MUST 按 `off` 处理。

#### Scenario: 公开设置包含该键
- **WHEN** 客户端请求 `GET /api/v1/settings/public`
- **THEN** 响应 MUST 包含 `leaderboard_mode`，其值 MUST 是三档之一
- **THEN** 注入负载的 schema 契约测试 MUST 覆盖该新增字段

#### Scenario: 前端派生布尔 flag
- **WHEN** `leaderboard_mode` 为 `anonymous`
- **THEN** 派生的布尔 flag MUST 为 `true`，侧边栏 MUST 显示入口

#### Scenario: 公开设置缺少该键
- **WHEN** 公开设置负载里没有 `leaderboard_mode`
- **THEN** 前端 MUST 按 `off` 处理并隐藏入口

### Requirement: anonymous 档下 Highlights 与 Insights 的数值裁剪
Leaderboard Mode（排行榜模式）为 `anonymous` 时，`highlights` 与 `insights` 里任何**他人的**或**站点级的**绝对量字段 MUST 缺席，MUST NOT 被清零后下发。可以下发的只有相对量与比率：`share_percent`（占全站该 Metric（排名指标）的百分比，0 到 100 的整数）、`lead_percent`（比第 2 名多出的整数百分比）、`relative_percent`（相对最高一天或峰值小时的 0 到 100 整数百分比）、`cache_hit_rate`（0 到 1 的比率）、`change_percent`（较上月的整数百分比，可为负）与 `peak_hour`——这些字段在 `named` 档下 MUST 同样下发，页面的条宽与色阶都靠它们。Highlights（趣味卡）领先者的身份 MUST 用榜单 Ordinal（行序号）假名表达，MUST NOT 用 Rank（名次）代替 Ordinal，MUST NOT 出现用户 id。查看者本人恰好是某张 Highlights 卡的领先者时，该卡的 `identity.kind` MUST 是 `self`，但其数值 MUST 仍按当前档位裁剪——该卡是给所有人看的同一份数据，不是「我的数据」。本人条目、`my_rank` 与「你的位置」的数值 MUST 不受本条影响，始终是真实值。

#### Scenario: 领先者用 Ordinal 假名
- **WHEN** `anonymous` 档下今日卷王在当前 Window（榜单窗口）+ Metric 榜单上的 Ordinal 是 3
- **THEN** `highlights.top_tokens.identity.kind` MUST 是 `anonymous`、`ordinal` MUST 是 `3`
- **THEN** 页面 MUST 渲染成「第 3 位」，响应 MUST NOT 包含其 `username`

#### Scenario: 领先者不在前 50
- **WHEN** `anonymous` 档下效率之星不在当前 Window + Metric 的前 50 内
- **THEN** `highlights.cache_king.ordinal` MUST 为 `null`
- **THEN** 页面 MUST 渲染成「榜外用户」，MUST NOT 用 Rank 顶替 Ordinal

#### Scenario: Highlights 只给占比与相对差
- **WHEN** `anonymous` 档下查看今日卷王与最勤快两张卡
- **THEN** 两张卡 MUST NOT 包含 `total_tokens` 或 `successful_requests` 字段
- **THEN** 两张卡 MUST 给出 `share_percent` 与 `lead_percent`，两者 MUST 都是整数

#### Scenario: 全站概况只给分档人数、命中率与峰值时段
- **WHEN** `anonymous` 档下该 Window 的全站 Total Tokens（总 tokens）为 21.4M、Participant Count（参与人数）为 137
- **THEN** `highlights.site` MUST NOT 包含 `total_tokens` 与 `successful_requests`
- **THEN** `highlights.site.participant_count` MUST 是分档字符串（如「100+」），并 MUST 给出 `cache_hit_rate` 与 `peak_hour`

#### Scenario: 模型热度只给占比
- **WHEN** `anonymous` 档下查看今日模型热度
- **THEN** `insights.models_today` 的每一项 MUST NOT 包含 `successful_requests`
- **THEN** 每一项 MUST 给出 `share_percent`

#### Scenario: 趋势只给相对最高日的百分比
- **WHEN** `anonymous` 档下查看近 30 天活跃度与近 14 天趋势
- **THEN** `insights.daily_30` 的每一项 MUST NOT 包含 `requests` 与 `total_tokens`
- **THEN** 每一项 MUST 给出 `relative_percent`，最高的一天 MUST 是 `100`

#### Scenario: 时段只给相对峰值的高度与峰值小时
- **WHEN** `anonymous` 档下查看今日时段分布
- **THEN** `insights.hourly_today` 的 24 个桶 MUST NOT 包含 `requests`
- **THEN** 每个桶 MUST 给出 `hour` 与 `relative_percent`，峰值桶的 `relative_percent` MUST 是 `100`

#### Scenario: 本月累计换成较上月百分比
- **WHEN** `anonymous` 档下查看「本月累计」
- **THEN** `insights.month` MUST NOT 包含 `total_tokens`
- **THEN** `insights.month.change_percent` MUST 给出较上月的整数百分比，上月的绝对量 MUST NOT 出现在响应里

#### Scenario: 今日缓存命中只给命中率
- **WHEN** `anonymous` 档下查看今日缓存命中
- **THEN** `insights.cache_today` MUST NOT 包含 `cache_read_tokens` 与 `input_tokens`
- **THEN** `insights.cache_today.cache_hit_rate` MUST 照常给出

#### Scenario: 你的位置的提示语不含他人绝对量
- **WHEN** `anonymous` 档下查看者排在第 12 名
- **THEN** 「你的位置」的提示语 MUST 只用相对第一名的百分比表达（如「相对第一名 2%，第 10 名是 3%」）
- **THEN** 提示语 MUST NOT 出现任何他人的 tokens 或请求数绝对值

#### Scenario: 查看者本人就是领先者
- **WHEN** `anonymous` 档下查看者本人是今日卷王
- **THEN** `highlights.top_tokens.identity.kind` MUST 是 `self`
- **THEN** 该卡的数值 MUST 仍然只有 `share_percent` 与 `lead_percent`，MUST NOT 因为是本人而放行绝对值

#### Scenario: named 档下同一份数据给绝对值
- **WHEN** 同一个 Window 在 `named` 档下被请求
- **THEN** Highlights、`models_today`、`daily_30`、`hourly_today`、`cache_today` 与 `month` MUST 给出对应的绝对量字段
- **THEN** `share_percent`、`lead_percent`、`relative_percent`、`cache_hit_rate` 与 `change_percent` MUST 仍然一并下发

### Requirement: anonymous 档下 Extremes 与新增 Insights 的数值裁剪
Leaderboard Mode（排行榜模式）为 `anonymous` 时，`highlights.extremes` 与新增的五块 Insights（洞察）里任何**他人的**或**站点级的**绝对量字段 MUST 缺席，MUST NOT 被清零后下发；可以下发的只有相对量与比率，它们在 `named` 档与 Preview（预览）下 MUST 同样下发。逐项如下：`night_owl` MUST 给 `night_share_percent`，`night_tokens` MUST 缺席；`rising` MUST 给 `change_percent`；`omnivore` MUST 给 `distinct_models`；`talker` MUST 给 `output_share_percent`；`max_single` MUST 给 `ratio_to_median`（相对全体参与者各自单次最大值之中位数的倍数），`max_single_tokens` MUST 缺席；`streak` MUST 给 `days`。`profiles` 的每一项 MUST 给模型占比，身份 MUST 用榜单 Ordinal（行序号）假名；`platforms_today` 的每一项 MUST 给 `share_percent`，`successful_requests` MUST 缺席；`weekly_rhythm` MUST 只给 0 到 4 的等级，两档完全相同；`composition_today` MUST 给四段的百分比，四个绝对 token 数 MUST 缺席；`cache_trend_14` MUST 给每天的 `cache_hit_rate`。Extremes（之最）与 `profiles` 的身份 MUST 与 Leaderboard Entry（榜单条目）同一套规则：MUST NOT 用 Rank（名次）代替 Ordinal，MUST NOT 出现用户 id，不在下发的 `entries` 内时 `ordinal` MUST 为 `null`。顶层 `viewer` 的数据 MUST 不受本条影响：它全部是查看者本人的数据，在任何档位下 MUST 都是真实值。

#### Scenario: 单次最大只给相对中位数的倍数
- **WHEN** `anonymous` 档下某用户的单次请求 tokens 是全站最大
- **THEN** `highlights.extremes.max_single` MUST NOT 包含 `max_single_tokens`
- **THEN** 该项 MUST 给出 `ratio_to_median`，页面 MUST 用「× N 倍于中位数」表达

#### Scenario: 进步之星在非今日窗口缺席
- **WHEN** `anonymous` 档下客户端请求 `window=month`
- **THEN** `highlights.extremes.rising` MUST 为 `null`
- **THEN** 系统 MUST NOT 改用上月基线凑出一个 `change_percent`

#### Scenario: 之最的领先者是榜外用户
- **WHEN** `anonymous` 档下夜猫子之最不在当前 Window（榜单窗口）+ Metric（排名指标）的 `entries` 内
- **THEN** `highlights.extremes.night_owl.identity.kind` MUST 是 `anonymous` 且 `ordinal` MUST 为 `null`
- **THEN** 页面 MUST 渲染成「榜外用户」，MUST NOT 用 Rank 顶替 Ordinal，也 MUST NOT 出现用户 id

#### Scenario: 夜猫子只给占比
- **WHEN** `anonymous` 档下查看夜猫子这张卡
- **THEN** 该项 MUST NOT 包含 `night_tokens`
- **THEN** 该项 MUST 给出 `night_share_percent`，是 0 到 100 的整数

#### Scenario: 平台分布与 Token 构成只给百分比
- **WHEN** `anonymous` 档下查看平台分布与今日 Token 构成
- **THEN** `insights.platforms_today` 的每一项 MUST NOT 包含 `successful_requests`，`insights.composition_today` MUST NOT 包含四段的绝对 token 数
- **THEN** 两块 MUST 各自给出百分比字段，页面的堆叠条 MUST 靠它们渲染

#### Scenario: 周内节奏两档相同
- **WHEN** 同一段数据分别在 `anonymous` 与 `named` 档下被请求
- **THEN** `insights.weekly_rhythm` 的 7 × 24 个值 MUST 完全相同，且 MUST 都是 0 到 4 的等级
- **THEN** 响应 MUST NOT 在任何档位下附带该块的平均请求数

#### Scenario: 模型偏好画像的身份按档位渲染
- **WHEN** `anonymous` 档下查看模型偏好画像
- **THEN** `insights.profiles` 每一项的身份 MUST 是 Ordinal 假名或 `self`，MUST NOT 包含 `username`
- **THEN** 每一项的模型占比 MUST 照常下发

#### Scenario: 你的统计在任何档位都真实
- **WHEN** `anonymous` 档下查看者查看「你的统计」
- **THEN** `viewer.rank_history`、`viewer.models`、`viewer.cache_hit_rate` 与 `viewer.avg_tokens_per_request` MUST 都是查看者本人的真实数据
- **THEN** 档位 MUST NOT 对 `viewer` 的任何字段做裁剪或降级

#### Scenario: named 档下同一份数据给绝对值
- **WHEN** 同一个 Window 在 `named` 档或 Preview 下被请求
- **THEN** `night_tokens`、`max_single_tokens`、`platforms_today` 的 `successful_requests` 与 `composition_today` 的四个绝对 token 数 MUST 都下发
- **THEN** 对应的百分比、比率与等级字段 MUST 仍然一并下发
