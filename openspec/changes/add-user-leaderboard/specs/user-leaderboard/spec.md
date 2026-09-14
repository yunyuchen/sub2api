## Purpose

定义 Leaderboard（排行榜）本体对登录用户的可见行为：用户侧接口的请求参数与响应结构、Window（榜单窗口）与 Metric（排名指标）的口径、Rank（名次）与 Ordinal（行序号）的算法、Leaderboard Entry（榜单条目）的上限、My Rank（我的名次）与 Participant Count（参与人数）、参与资格，以及页面的路由、入口与五种非正常态。三档暴露程度的差异见 `leaderboard-mode`，数据来源见 `leaderboard-snapshot`。

## ADDED Requirements

### Requirement: Leaderboard 接口的参数取值与默认值
系统 SHALL 提供 `GET /api/v1/leaderboard`，接受查询参数 `window`（`today` / `week` / `month`）与 `metric`（`total_tokens` / `successful_requests` / `cost`）。两个参数 MUST 都有默认值：`window` 默认 `today`，`metric` 默认 `total_tokens`。取值不在枚举内时 MUST 返回 400 且 MUST NOT 回落到默认值。接口 MUST NOT 接受自定义起止日期、「全部时间」窗口或榜单长度参数。Leaderboard Mode（排行榜模式）guard MUST 先于参数校验执行：`off` 档下普通用户的任何请求（含携带非法参数的请求）MUST 一律返回 404，MUST NOT 返回 400。

#### Scenario: 不带任何参数
- **WHEN** 客户端请求 `GET /api/v1/leaderboard`
- **THEN** 系统 MUST 按 `window=today`、`metric=total_tokens` 返回
- **THEN** 响应里的 `window` 与 `metric` MUST 回显实际生效的取值

#### Scenario: 非法 window
- **WHEN** 请求携带 `window=year`
- **THEN** 系统 MUST 返回 400
- **THEN** 系统 MUST NOT 按 `today` 返回数据

#### Scenario: 非法 metric
- **WHEN** 请求携带 `metric=money`
- **THEN** 系统 MUST 返回 400
- **THEN** 系统 MUST NOT 按 `total_tokens` 返回数据

#### Scenario: 按 Cost 排序
- **WHEN** 请求携带 `metric=cost`
- **THEN** 系统 MUST 按 Cost（消费金额）返回，响应里的 `metric` MUST 回显 `cost`
- **THEN** 系统 MUST NOT 返回 400

#### Scenario: 携带自定义日期参数
- **WHEN** 请求额外携带 `start_date` / `end_date` 之类的参数
- **THEN** 这些参数 MUST NOT 改变 Window 的边界
- **THEN** 返回的仍 MUST 是 `window` 指定的固定窗口

#### Scenario: off 档下的非法参数
- **WHEN** Leaderboard Mode 为 `off`，普通用户请求 `GET /api/v1/leaderboard?window=year`
- **THEN** 系统 MUST 返回 404，MUST NOT 返回 400
- **THEN** 响应 MUST NOT 透露该路由存在

### Requirement: Leaderboard 响应的字段集合与结构化身份
响应体 MUST 包含 `window`、`metric`、`mode`、`preview`、`timezone`、`status`、`stale`、`snapshot_updated_at`、`participant_count`、`entries`、`entries_suppressed`、`my_rank` 这些字段。`status` 的取值 MUST 只有 `ready` 与 `computing`：`computing` 表示 Snapshot（榜单快照）缺失，此时 `snapshot_updated_at` MUST 为 `null`、`entries` MUST 是空数组。`stale` MUST 是布尔值，`snapshot_updated_at` 距当前时刻超过 15 分钟时 MUST 为 `true`。`entries_suppressed` MUST 是布尔值，仅在 `anonymous` 档因参与人数过少而不下发条目时为 `true`（见 `leaderboard-mode`）。每个 Leaderboard Entry MUST 包含 `rank`、`ordinal`、`identity`、`is_self` 以及当前档位允许的数值字段。`identity` MUST 是结构化对象 `{kind: self | anonymous | named, username?, avatar_url?}`；后端 MUST NOT 拼接 Display Name（展示名）字符串下发。`avatar_url` 是可选的头像小图（见「榜单身份的头像字段」），MUST 只在 `kind` 为 `named` 时可能出现，`self` 与 `anonymous` 的身份里 MUST NOT 包含它。Display Name 的文案 MUST 由前端 i18n 渲染：`self` 渲染为查看者本人的 `username`（去空白后为空时才回退「当前用户」，沿用 `channelMonitorV2.currentUser` 的术语）、`anonymous` 渲染为「第 Ordinal 位」、`named` 直接使用 `username`；响应与页面 MUST NOT 出现字面量 `Me`。这四条分支 MUST 只有一处实现（`frontend/src/components/user/leaderboard/displayName.ts`），榜单条目、Highlights、Extremes 与模型画像 MUST 复用它。`self` 取的是查看者自己的资料，MUST NOT 对它套用 Named Participation 的 `username` 校验——那条校验挡的是「把别人的名字推给第三方看」，而本人行只有本人看得见。

#### Scenario: 查看者本人的条目
- **WHEN** 查看者本人进入前 50 并出现在 `entries` 中
- **THEN** 该条目的 `identity.kind` MUST 为 `self` 且 MUST NOT 携带 `username`
- **THEN** 该条目的 `is_self` MUST 为 `true`

#### Scenario: 实名条目
- **WHEN** 某条目对应的用户以实名形态展示
- **THEN** 该条目的 `identity.kind` MUST 为 `named` 且 `identity.username` MUST 是该用户的 `username`

#### Scenario: 双语渲染
- **WHEN** 同一份响应分别在中文与英文界面下渲染
- **THEN** `self` 条目 MUST 分别显示为「当前用户」与 `Current user`
- **THEN** 后端 MUST NOT 因语言不同而返回不同的身份字段

### Requirement: Window 的取值与站点时区边界
Window MUST 只有今日 / 本周 / 本月三个取值，边界一律按站点时区计算：今日用 `StartOfDay`、本周用 `StartOfWeek`（周从周一开始）、本月用 `StartOfMonth`。边界 MUST NOT 按查看者浏览器时区计算，MUST NOT 提供自定义起止日期，也 MUST NOT 提供「全部时间」窗口。响应 MUST 带上计算所用的时区名，页面 MUST 把它展示出来。

#### Scenario: 本周从周一起算
- **WHEN** 站点时区当天是周三，查看者请求 `window=week`
- **THEN** 窗口起点 MUST 是本周周一的站点时区零点
- **THEN** 上周日的用量 MUST NOT 计入本周

#### Scenario: 跨零点
- **WHEN** 站点时区刚跨过零点进入新的一天
- **THEN** `window=today` 的榜单 MUST 只包含新一天的用量
- **THEN** 前一天的数值 MUST NOT 出现在今日榜中

#### Scenario: 查看者与站点不在同一时区
- **WHEN** 两个处在不同浏览器时区的用户在同一时刻请求同一个 Window
- **THEN** 两人 MUST 看到同一张榜、同一个窗口起点
- **THEN** 响应 MUST 带同一个 `timezone` 值

### Requirement: Metric 有三项：Total Tokens、Successful Requests 与 Cost
Metric MUST 只有三项：Total Tokens（总 tokens）= `input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens`，与用户仪表盘、管理端 User Breakdown（用户用量明细）口径一致；Successful Requests（成功请求数）= `actual_cost > 0` 的请求数，失败占位记录 MUST NOT 计入；Cost（消费金额）= 该 Window（榜单窗口）内 `actual_cost` 之和（USD），与用户自己的用量页「花费」同一个口径，MUST NOT 改用 `total_cost` 之类的其它金额口径。默认 Metric MUST 是 Total Tokens。每个 Leaderboard Entry MUST 同时给出三个 Metric 的值（在 `anonymous` 档下按该档规则替换为相对值），按选中的 Metric 排序并在页面上高亮该列。页面 MUST 注明 Successful Requests 只计成功落账的请求。

#### Scenario: 失败占位记录不计入
- **WHEN** 某用户在窗口内产生了 100 条 `actual_cost = 0` 的失败记录与 10 条成功记录
- **THEN** 该用户的 Successful Requests MUST 为 10
- **THEN** 刷失败请求 MUST NOT 提高其在 `successful_requests` 榜上的位置

#### Scenario: cache 类 tokens 计入
- **WHEN** 某用户的用量全部来自 `cache_read_tokens`
- **THEN** 这些 tokens MUST 计入其 Total Tokens
- **THEN** 该数值 MUST 与用户仪表盘上同一窗口的 Total Tokens 口径一致

#### Scenario: 按 Successful Requests 排序
- **WHEN** 请求携带 `metric=successful_requests`
- **THEN** `entries` MUST 按 Successful Requests 降序排列
- **THEN** 每个条目 MUST 仍然给出 Total Tokens 与 Cost（消费金额）的值，页面 MUST 高亮 Successful Requests 列

#### Scenario: Cost 与成功落账口径同源
- **WHEN** 某用户在窗口内产生了 100 条 `actual_cost = 0` 的失败记录与 10 条成功记录
- **THEN** 其 Cost MUST 只是那 10 条的 `actual_cost` 之和
- **THEN** 该数值 MUST 与用户自己的用量页同一窗口的「花费」口径一致

#### Scenario: 其它金额口径不可达
- **WHEN** 客户端尝试读取 `total_cost` 之类的其它金额字段
- **THEN** 响应体 MUST NOT 包含它
- **THEN** 响应体里唯一的金额 MUST 只有 Cost（`actual_cost` 之和）这一个口径

### Requirement: Rank 是竞争排名
Rank（名次）MUST 等于同一 Window + Metric 下该 Metric 值严格高于自己的合格用户数加一。数值相同的用户 MUST 得到相同的 Rank，其后的 Rank MUST 跳号（1、1、3）。Leaderboard Entry 与 My Rank MUST 使用同一条规则，两处 MUST NOT 互相矛盾。系统 MUST NOT 把 Rank 取成行下标。

#### Scenario: 并列名次
- **WHEN** 前两名用户在选中 Metric 上的数值完全相同，第三名严格低于他们
- **THEN** 前两个条目的 `rank` MUST 都是 `1`
- **THEN** 第三个条目的 `rank` MUST 是 `3`，MUST NOT 是 `2`

#### Scenario: My Rank 与榜内名次一致
- **WHEN** 查看者出现在 `entries` 中
- **THEN** `my_rank.rank` MUST 与该条目的 `rank` 完全相同

#### Scenario: 切换 Metric 后名次变化
- **WHEN** 同一查看者先后请求两个不同的 `metric`
- **THEN** 两次的 `rank` MUST 各自按对应 Metric 重新计算
- **THEN** 系统 MUST NOT 沿用另一个 Metric 的名次

### Requirement: Ordinal 只用于匿名形态的展示名
Ordinal（行序号）MUST 是当前 Window + Metric 下 `entries` 的连续序号，从 1 起、唯一、不跳号。Ordinal MUST NOT 与 Rank 混用：它只用于匿名形态的 Display Name「第 Ordinal 位」。切换 Metric 时同一用户的 Ordinal 可能变化。用户 id MUST NOT 出现在 Display Name 里，「User #id」这类兜底 MUST NOT 使用。

#### Scenario: 并列时 Ordinal 仍连续
- **WHEN** 前两个条目的 `rank` 都是 `1`
- **THEN** 它们的 `ordinal` MUST 分别是 `1` 与 `2`
- **THEN** 页面 MUST NOT 出现两行「第 1 位」

#### Scenario: 切换 Metric 后 Ordinal 变化
- **WHEN** 同一用户在 `total_tokens` 榜上的 `ordinal` 是 `7`，在 `successful_requests` 榜上排在更靠前的位置
- **THEN** 其在后一个榜上的 `ordinal` MUST 按新顺序重新分配
- **THEN** 系统 MUST NOT 为该用户保留跨 Window 或跨 Metric 稳定的代号

#### Scenario: 展示名不含用户 id
- **WHEN** 某个条目以匿名形态展示
- **THEN** 其 Display Name MUST 只由 Ordinal 构成
- **THEN** 响应与页面 MUST NOT 出现该用户的 id

### Requirement: Leaderboard Entry 至多 50 条
`entries` MUST 至多包含 50 条，取选中 Metric 降序的前 50 名。系统 MUST NOT 提供榜单长度选择器或分页参数。被截断的用户 MUST 仍能通过 My Rank 得到自己的 Rank。

#### Scenario: 参与者多于 50
- **WHEN** 该 Window 内有 3000 个有用量的合格用户
- **THEN** `entries` MUST 恰好包含 50 条
- **THEN** 第 51 名及之后的用户 MUST NOT 出现在 `entries` 中

#### Scenario: 恰好第 50 名
- **WHEN** 某用户的 Rank 恰好是 50 且榜内没有并列把他挤出前 50 条
- **THEN** 该用户 MUST 出现在 `entries` 的最后一条，`ordinal` MUST 是 `50`

#### Scenario: 第 50 与第 51 条并列
- **WHEN** 按选中 Metric 排序后第 50 条与第 51 条的数值相同
- **THEN** `entries` MUST 仍只返回 50 条
- **THEN** 被截断的那名用户的 `my_rank.rank` MUST 与榜内最后一条的 `rank` 相同

#### Scenario: 参与者少于 50
- **WHEN** 该 Window 内只有 12 个有用量的合格用户
- **THEN** `entries` MUST 包含 12 条，MUST NOT 补空行

### Requirement: My Rank 与 Participant Count
`my_rank` MUST 在查看者不进前 50 时也返回，字段包含 `rank`、`total_tokens`、`successful_requests`、`cost`，且 MUST 始终是查看者的真实数值。查看者在该 Window 内没有任何用量时 `my_rank` MUST 为 `null`，页面 MUST 提示「本窗口暂无用量」而 MUST NOT 给出一个名次。`participant_count` MUST 是该 Window 内有过任何用量的合格用户总数，作为 My Rank 的分母展示；其表达形式随档位而定：`named` 档与 Preview（预览）下 MUST 是精确整数，`anonymous` 档下 MUST 是分档字符串，取值规则见 `leaderboard-mode`。查看者进入前 50 时，对应的 Leaderboard Entry MUST 标记 `is_self` 并被页面高亮。

#### Scenario: 查看者不在前 50
- **WHEN** 查看者在该 Window 有用量但 Rank 是 137
- **THEN** `my_rank.rank` MUST 是 `137`
- **THEN** 页面 MUST 把它与 `participant_count` 一起展示为「137 / 参与人数」的形式

#### Scenario: 零用量
- **WHEN** 查看者在该 Window 内没有任何用量
- **THEN** `my_rank` MUST 为 `null`
- **THEN** 页面 MUST 显示「本窗口暂无用量」，MUST NOT 把查看者排到末位并给出一个巨大的名次

#### Scenario: 查看者在前 50 内
- **WHEN** 查看者的 Rank 是 8
- **THEN** `entries` 中对应条目的 `is_self` MUST 为 `true` 且被页面高亮
- **THEN** `my_rank.rank` MUST 也是 `8`

#### Scenario: 本人数值始终真实
- **WHEN** 当前档位对他人的数值做了降级处理
- **THEN** `my_rank` 与 `is_self` 条目的数值 MUST 仍是真实的绝对值

#### Scenario: participant_count 的类型随档位变化
- **WHEN** 同一个 Window 分别在 `named` 与 `anonymous` 档下被请求
- **THEN** `named` 档的 `participant_count` MUST 是整数
- **THEN** `anonymous` 档的 `participant_count` MUST 是分档字符串，MUST NOT 是整数

### Requirement: 参与资格按响应时刻的用户状态判定
参与资格 MUST 满足：管理员账号照常参与；`users.status = disabled` 的用户与已软删除（`deleted_at` 非空）的用户 MUST 被排除。时点口径 MUST 是：聚合时刻已不合格者 MUST NOT 进入 Snapshot（榜单快照），因而不计入 `participant_count`；响应时刻新变为不合格者 MUST 在渲染时从 `entries` 剔除，而 `participant_count` 与 Rank 仍是 Snapshot 的冻结值，直到下一轮作业生效（见 `leaderboard-snapshot`）。资格 MUST 按响应时刻的 `users` 表当前状态判定，MUST NOT 冻结在 Snapshot 里。系统 MUST NOT 提供「不上榜」这样的退出开关——身份的可见性由 Named Participation（昵称展示）承担。

#### Scenario: 管理员参与排名
- **WHEN** 某管理员在该 Window 内有用量
- **THEN** 该管理员 MUST 与普通用户一样出现在榜单条目中并计入 Participant Count
- **THEN** 系统 MUST NOT 因为角色是管理员而对其数值做特殊处理

#### Scenario: 被禁用的用户
- **WHEN** 某用户的 `status` 是 `disabled`
- **THEN** 该用户 MUST NOT 出现在 `entries` 中
- **THEN** 下一轮 Snapshot 重建后该用户 MUST NOT 计入 `participant_count`

#### Scenario: 已软删除的用户
- **WHEN** 某用户的 `deleted_at` 非空
- **THEN** 该用户 MUST NOT 出现在 `entries` 中；下一轮 Snapshot 重建后 MUST NOT 计入 `participant_count`

#### Scenario: 没有退出排行的开关
- **WHEN** 某用户把 Named Participation 关掉了
- **THEN** 该用户 MUST 仍然参与排名并计入 `participant_count`
- **THEN** 其 Display Name MUST 为匿名形态

### Requirement: 前三名徽章按 Rank 值判定
页面 MUST 按条目的 `rank` 值小于等于 3 判定前三名的强调形态，MUST NOT 按行下标或 `ordinal` 判定。重设计后的独立页不再复用 `MonitorRankBadge.vue`（它属于另一套皮肤，且只有一种形态，表达不了「前三名」与「其余名次」两类渲染）。**强调形态由当前皮肤决定**——奖牌、方括号徽标、补零序号、强调色与字重都可以，它随皮肤改版而变；判定规则 MUST 不随皮肤改版而变，任何皮肤下都 MUST 只看 `rank` 值。

#### Scenario: 并列第一
- **WHEN** 前两个条目的 `rank` 都是 `1`
- **THEN** 两行 MUST 都显示第一名徽章

#### Scenario: 跳号后的第四名
- **WHEN** 某条目的 `rank` 是 `4`（因前面并列而跳过了 `3`）
- **THEN** 该行 MUST NOT 显示徽章
- **THEN** 徽章 MUST NOT 因为它是第三行而被渲染

### Requirement: 加载、空、抑制、正在计算与陈旧五种非正常态
页面 MUST 分别处理五种非正常态并各有文案：请求进行中的加载态；Snapshot 已就绪但该 Window 内没有任何合格用户的空态；`anonymous` 档下参与人数过少而不下发条目的抑制态（见 `leaderboard-mode`）；Snapshot 尚未生成时的「正在计算」态；Snapshot 更新时间超过 15 分钟未推进时的陈旧警告态。响应 MUST 通过 `status`、`stale` 与 `entries_suppressed` 三个字段给出可判定的显式信号，抑制态与空态 MUST 能被明确区分，前端 MUST NOT 靠「entries 是空数组」反推处于哪一种状态。「正在计算」MUST NOT 表现为空榜或 500。陈旧态 MUST 仍然展示数据，但 MUST 显式给出警告而不是静默展示旧数据。页面 MUST 常驻展示 `snapshot_updated_at` 与计算时区。

#### Scenario: Snapshot 尚未生成
- **WHEN** 首次开启 Leaderboard Mode 后后台作业还没跑完第一轮
- **THEN** 响应的 `status` MUST 是 `computing`，`snapshot_updated_at` MUST 为 `null`
- **THEN** 页面 MUST 显示「正在计算」文案，MUST NOT 显示空榜或报错

#### Scenario: 数据陈旧
- **WHEN** `snapshot_updated_at` 距当前时刻超过 15 分钟
- **THEN** 响应的 `stale` MUST 为 `true` 且 `entries` MUST 仍返回已有数据
- **THEN** 页面 MUST 在展示数据的同时显示陈旧警告

#### Scenario: 窗口内无人有用量
- **WHEN** Snapshot 已就绪但该 Window 内没有任何合格用户产生用量
- **THEN** `status` MUST 是 `ready`、`entries_suppressed` MUST 为 `false`、`entries` MUST 是空数组
- **THEN** `participant_count` MUST 表达 0 个参与者（`named` 档为整数 `0`，`anonymous` 档为对应的分档字符串）
- **THEN** 页面 MUST 显示空态文案而不是「正在计算」

#### Scenario: 人数过少的抑制态
- **WHEN** `anonymous` 档下该 Window 的 Participant Count 少于 5
- **THEN** `status` MUST 是 `ready`、`entries_suppressed` MUST 为 `true`、`entries` MUST 是空数组
- **THEN** 页面 MUST 显示「参与人数过少，暂不展示榜单」而 MUST NOT 显示空态文案

#### Scenario: 正常新鲜
- **WHEN** `snapshot_updated_at` 距当前时刻在 15 分钟以内
- **THEN** 响应的 `status` MUST 是 `ready`、`stale` MUST 为 `false`
- **THEN** 页面 MUST 正常展示榜单与更新时间，MUST NOT 显示陈旧警告

### Requirement: 独立路由与侧边栏入口
系统 SHALL 新增用户侧路由 `/leaderboard`（`requiresAuth: true`，带 `meta.titleKey`），侧边栏新增一项指向它，`hideInSimpleMode` 与 `/usage` 保持一致。侧边栏入口 MUST 随「Leaderboard Mode 不为 `off`」的派生布尔 flag 显隐。路由守卫 MUST fail-closed：先确保公开设置已加载，仅在公开设置已成功加载、明确为 `off` 且查看者不是管理员时拦截；公开设置瞬时加载失败时 MUST 放行并交给后端兜底。`off` 档下侧边栏入口对所有角色隐藏，管理员的直链访问 MUST 被放行以进入 Preview（预览）。Leaderboard MUST NOT 做成 `/usage` 的一个 tab，也 MUST NOT 在仪表盘新增卡片。

#### Scenario: 模式开启时入口可见
- **WHEN** Leaderboard Mode 为 `anonymous` 或 `named`
- **THEN** 侧边栏 MUST 出现 Leaderboard 入口
- **THEN** 访问 `/leaderboard` MUST 正常进入页面

#### Scenario: 模式为 off 时入口隐藏
- **WHEN** Leaderboard Mode 为 `off`
- **THEN** 侧边栏 MUST NOT 出现 Leaderboard 入口（管理员也不例外）
- **THEN** 普通用户直接访问 `/leaderboard` MUST 被路由守卫拦截
- **THEN** 管理员直接访问 `/leaderboard` MUST 被放行并进入 Preview

#### Scenario: 公开设置加载失败
- **WHEN** 公开设置尚未成功加载，用户直接访问 `/leaderboard`
- **THEN** 路由守卫 MUST NOT 因读不到设置而误判放行为正常访问
- **THEN** 守卫 MUST 把最终判定交给后端，由接口按 Leaderboard Mode 返回结果

### Requirement: 用户侧中间件链与 Heavy 限流
`GET /api/v1/leaderboard` MUST 注册在用户侧 `authenticated` 组内，依次经过 `jwtAuth`、`BackendModeUserGuard`、`Global()` 限流与审计日志，并额外应用 `panelRateLimiter.Heavy()`（与 `/usage/*`、`channel-monitor-v2` 一致）与 Leaderboard Mode guard。系统 MUST NOT 为 Leaderboard 新增管理端路由：`off` 下的 Preview（预览）走同一个用户端接口。

#### Scenario: 未认证请求
- **WHEN** 请求不携带有效的 JWT
- **THEN** 请求 MUST 被 `jwtAuth` 拒绝
- **THEN** 请求 MUST NOT 触达 Leaderboard 的任何查询逻辑

#### Scenario: 触发 Heavy 限流
- **WHEN** 单个用户在短时间内密集请求该接口并超过 `Heavy()` 的阈值
- **THEN** 系统 MUST 按与 `/usage/*` 一致的方式限流
- **THEN** 被限流的请求 MUST NOT 读取 Snapshot

#### Scenario: 管理员使用同一个接口
- **WHEN** 管理员访问 Leaderboard
- **THEN** 管理员 MUST 使用与普通用户相同的 `GET /api/v1/leaderboard`
- **THEN** 系统 MUST NOT 另外暴露一个管理端专用的 Leaderboard 路由

### Requirement: 响应包含 Highlights 与 Insights 两个顶层字段
响应体 MUST 在既有字段之外包含 `highlights` 与 `insights` 两个顶层字段。`highlights` MUST 是该 Window（榜单窗口）的四块 Highlights（趣味卡）数据 `{top_tokens, top_requests, cache_king, site}`；`top_tokens`、`top_requests`、`cache_king` 在无人满足条件时 MUST 为 `null`，MUST NOT 用零值对象顶替。`insights` MUST 是与 Window 无关的站点级 Insights（洞察）`{models_today, daily_30, hourly_today, cache_today, month}`；任一区块的数据源缺失时该区块 MUST 为 `null`，MUST NOT 用 0 填充，也 MUST NOT 让整个响应失败。`status` 为 `computing` 时 `highlights` MUST 为 `null`。Highlights 里的身份 MUST 与 Leaderboard Entry（榜单条目）用同一个结构化形态 `{kind, username?, avatar_url?}`（`avatar_url` 的下发条件与榜单条目完全一致，MUST NOT 另立一套判定），并额外带一个 `ordinal`：该用户不在当前 Window + Metric（排名指标）的前 50 内时 `ordinal` MUST 为 `null`。Highlights 与 Insights MUST NOT 使响应出现 `user_id` 或邮箱；其中唯一允许出现的金额 MUST 是 `highlights.site.cost`（该 Window 的 `actual_cost` 之和，`named` 档与 Preview 才给），MUST NOT 出现其它金额口径，也 MUST NOT 为 Cost（消费金额）新增一张 Highlights 卡。Cache Hit Rate（缓存命中率）MUST 以 `cache_hit_rate` 表达，取值范围是 0 到 1；对应窗口的 `input_tokens + cache_read_tokens` 为 0 时该字段 MUST 缺席，MUST NOT 记成 0。

#### Scenario: Highlights 的领先者不在前 50
- **WHEN** 某个 Window 的效率之星在当前 Metric 的榜单上排在第 51 名之后
- **THEN** `highlights.cache_king.ordinal` MUST 为 `null`
- **THEN** 页面 MUST 把该卡的身份渲染成「榜外用户」，MUST NOT 编一个序号出来

#### Scenario: 快照尚未生成
- **WHEN** 响应的 `status` 是 `computing`
- **THEN** `highlights` MUST 为 `null`
- **THEN** 页面 MUST 把四张 Highlights 卡渲染成骨架，MUST NOT 渲染 0 值卡

#### Scenario: 无人达到效率之星的门槛
- **WHEN** 该 Window 内没有任何用户的 Successful Requests（成功请求数）超过门槛
- **THEN** `highlights.cache_king` MUST 为 `null`
- **THEN** 页面 MUST 隐藏该卡，MUST NOT 展示一个命中率为 0 的用户

#### Scenario: 某个 Insights 区块的数据源缺失
- **WHEN** 站点关闭了仪表盘预聚合作业，`daily_30` 与 `hourly_today` 的来源表没有数据
- **THEN** `insights.daily_30` 与 `insights.hourly_today` MUST 为 `null`
- **THEN** 榜单本体、Highlights 与 `insights.models_today` MUST 照常返回

#### Scenario: Highlights 不引入新的身份字段
- **WHEN** 检查任意档位下的完整响应体
- **THEN** `highlights` 与 `insights` 里 MUST NOT 出现 `user_id` 或邮箱，金额 MUST 只可能是 `highlights.site.cost`
- **THEN** Highlights 里唯一可能出现的身份信息 MUST 只有 `identity.username`

### Requirement: Leaderboard 是套站点外壳的应用内页
Leaderboard（排行榜）页面 MUST 是应用内页：MUST 套站点的 `AppLayout` 外壳，与站点其它用户页一致，侧边栏与站点顶栏由外壳提供。页面 MUST NOT 另起一条页面私有的站点导航（品牌条、站内链接组、页内跳转按钮都算），也 MUST NOT 提供「返回仪表盘」这类页内跳转入口——页面间导航由 `AppLayout` 的侧边栏承担，当前页标记同理。页面自己的顶部工具区 MUST 只保留与本页数据有关的部分：当前生效的 Window（榜单窗口）与 Metric（排名指标）切换，以及 Snapshot（榜单快照）更新时间与 Leaderboard Mode（排行榜模式）的回显。页面 MUST 是深色专属：配色 MUST NOT 跟随站点的明暗（站点处于亮色时呈现为「浅色外壳 + 深色内容面板」，这是预期结果），页面 MUST NOT 读写站点的主题状态（`document.documentElement` 上的 `dark` class 与 `localStorage` 里的 `theme`），MUST NOT 另起一套页面私有的主题状态，也 MUST NOT 只按 `prefers-color-scheme` 判定。页面的视觉 token MUST 作用在承载皮肤的那个页面根节点这一层而不是 `:root`，样式 MUST NOT 进入全局样式入口；该节点是外壳内容区里的一块内容面板，MUST NOT 占满视口，其装饰层 MUST 限制在自己的盒子内，MUST NOT 覆盖外壳的侧边栏或顶栏。侧边栏入口 MUST 保留（见「独立路由与侧边栏入口」）；路由、路由守卫与 `hideInSimpleMode` MUST NOT 因为页面形态改变而改变。工具区的**形态**由当前皮肤决定，细则见「Leaderboard 页面的皮肤作用域、顶部工具区与区块标题」——v1 写在本条里的斜体 tagline 与环境动效开关、v3 写在本条里的「工具区提供主题开关」与「主题跟随 `html.dark`」、以及 v4 写在本条里的「全屏独立页、不套 `AppLayout`、自带站点顶栏与『返回仪表盘』入口」，MUST NOT 再作为实现依据（2026-09-14 用户指令「做成不用跳转的内页」把页面改回应用内页，深色专属不变）。

#### Scenario: 页面在应用外壳内
- **WHEN** 用户从侧边栏进入 `/leaderboard`
- **THEN** 页面 MUST 渲染在 `AppLayout` 外壳内，站点侧边栏 MUST 仍然可见
- **THEN** 用户 MUST 能直接从侧边栏切到其它页面，页面 MUST NOT 提供额外的「返回仪表盘」入口或页面私有的站点导航

#### Scenario: 页面恒为深色
- **WHEN** 站点处于亮色主题（`html` 上没有 `dark` class）时用户进入 `/leaderboard`
- **THEN** 页面 MUST 仍然渲染深色，MUST NOT 出现亮色配色，页面上 MUST NOT 有主题开关
- **THEN** 用户从侧边栏切回仪表盘后仪表盘 MUST 仍然是亮色——本页 MUST NOT 改变站点主题，MUST NOT 写 `html.dark` 或 `localStorage` 里的 `theme`；外壳（侧边栏与顶栏）MUST NOT 被本页的深色面板覆盖

#### Scenario: 页面样式不外溢
- **WHEN** 用户从 Leaderboard 跳到其它任意页面
- **THEN** 其它页面的视觉 MUST NOT 被 Leaderboard 的 token 改变
- **THEN** Leaderboard 的样式 MUST NOT 被注册进全局样式入口

#### Scenario: 侧边栏入口仍然存在
- **WHEN** Leaderboard Mode（排行榜模式）为 `anonymous` 或 `named`
- **THEN** 侧边栏 MUST 仍然显示 Leaderboard 入口
- **THEN** 入口的显隐规则 MUST 与页面形态改动之前完全一致

### Requirement: 非正常态在独立页上的形态
四种非正常态 MUST 按已确认的 mockup 渲染。Preview（预览）横幅 MUST 出现在页面标题之上（页面标题的形态随皮肤改版而变，横幅与它的相对位置不变）。「正在计算」态 MUST 把 Highlights（趣味卡）与榜单一起渲染成骨架，并配「后台每 5 分钟重建」的说明文案，MUST NOT 渲染空卡或 0 值。陈旧态 MUST 用警告横幅提示并照常展示数据。抑制态 MUST 只显示「你的位置」条与一行说明，MUST NOT 显示任何其他条目。状态判定 MUST 只依据 `preview`、`status`、`stale`、`entries_suppressed` 四个字段，MUST NOT 靠「entries 是空数组」反推。Highlights 与 Insights（洞察）的各区块 MUST 各自独立降级：某个区块为 `null` 时只隐藏该区块，MUST NOT 影响榜单本体的渲染。

#### Scenario: 管理员在 off 档下的预览横幅
- **WHEN** `preview` 为 `true`
- **THEN** 页面 MUST 在页面标题之上显示「预览模式：普通用户不可见」横幅
- **THEN** 页面其余部分 MUST 按最开放档位正常渲染

#### Scenario: 正在计算时的骨架
- **WHEN** `status` 为 `computing`
- **THEN** 四张 Highlights 卡与榜单 MUST 都渲染成骨架并配说明文案
- **THEN** 页面 MUST NOT 显示空榜、0 值或报错

#### Scenario: 陈旧时仍展示数据
- **WHEN** `stale` 为 `true`
- **THEN** 页面 MUST 显示陈旧警告横幅
- **THEN** 榜单、Highlights 与 Insights MUST 照常展示已有数据

#### Scenario: 抑制态只剩本人行
- **WHEN** `entries_suppressed` 为 `true`
- **THEN** 页面 MUST 只显示「你的位置」条与一行说明
- **THEN** 页面 MUST NOT 显示任何其他榜单条目

#### Scenario: 单个区块降级不牵连榜单
- **WHEN** `insights.cache_today` 为 `null` 而其余字段正常
- **THEN** 页面 MUST 只隐藏今日缓存命中这一块
- **THEN** 榜单、Highlights 与其余 Insights 区块 MUST 照常渲染

### Requirement: Leaderboard 页面的皮肤作用域、顶部工具区与区块标题
Leaderboard（排行榜）页面的视觉 MUST 由一套页面私有的皮肤渲染，且 MUST 满足以下与皮肤无关的约束。页面根节点 MUST 带一个承载视觉 token 的 class，配色 MUST 是深色专属的一套（v1–v3 的「明暗两套配色各自完整、明暗由 `html.dark` 推导」自 2026-09-14 的 spool 内页皮肤起 MUST NOT 再作为实现依据）；根节点 MUST NOT 再带明暗修饰 class，页面 MUST NOT 跟随站点明暗，MUST NOT 读写 `html.dark` 与 `localStorage` 里的 `theme`，MUST NOT 另起一套页面私有的主题状态，也 MUST NOT 只按 `prefers-color-scheme` 判定。视觉 token MUST 仍然定义在页面根节点这一层而不是 `:root`，样式 MUST NOT 进入全局样式入口；页面 MUST NOT 依赖站点 CSP 未放行的第三方字体域名；若用到非系统字体则 MUST 自托管（v3 皮肤自托管了两个 woff2，spool 内页皮肤自 2026-09-14 起改用与首页一致的系统字体栈、不再加载任何字体文件，因此本条对它以「不得引用第三方字体域名」的形式成立）。页面顶部 MUST 有一条工具区，至少包含当前生效的 Window（榜单窗口）与 Metric（排名指标）切换、当前 Leaderboard Mode（排行榜模式）与 Snapshot（榜单快照）更新时间（v1–v4 写在本条里的「站点标识」「计算时区」与「『返回仪表盘』入口」三项自 2026-09-14 的方向修订起 MUST NOT 再作为实现依据：站点标识与页面间导航归 `AppLayout` 的侧边栏，计算时区改在页脚回显一次）；Window 与 Metric 的切换在页面上出现多处时 MUST 共用同一份状态与同一组请求参数，MUST NOT 各自维护。页面标题 MUST 回显当前生效的 Window，工具区 MUST 回显当前生效的 Window 与 Metric。每个区块 MUST 有自己的标题与一句口径说明。区块标题、口径说明、卡片标签、提示语与状态文案 MUST 走 i18n 且 zh / en 齐备；只有技术字面量——字段名、运算式（如 `successful_requests = actual_cost > 0`）、时区名，以及 Window 与 Metric 的英文取值——MUST NOT 被翻译。页面 MUST NOT 引入不可关闭的常驻动效；若皮肤提供了动效开关，其状态 MUST 持久化、默认值 MUST 取「用户没有要求减少动效」。`prefers-reduced-motion: reduce` 时页面 MUST 没有任何动画与数字滚动，内容 MUST 照常完整渲染。前三名的强调形态与它的判定规则见「前三名徽章按 Rank 值判定」，MUST NOT 在皮肤层改变判定。

#### Scenario: 页面顶部是工具区
- **WHEN** 用户进入 `/leaderboard`
- **THEN** 页面顶部 MUST 有一条工具区，回显当前的 Window、Metric、Leaderboard Mode 与 Snapshot 更新时间（计算时区在页脚回显）
- **THEN** 页面 MUST 仍然渲染在 `AppLayout` 外壳内，MUST NOT 提供「返回仪表盘」入口，MUST NOT 另起页面私有的站点导航

#### Scenario: 标题与工具区回显当前参数
- **WHEN** 请求生效的参数是 `window=week`、`metric=successful_requests`
- **THEN** 页面标题 MUST 反映 `week` 这个 Window，工具区的 Metric 切换 MUST 指向 `successful_requests`
- **THEN** 页面上回显的取值 MUST 与响应回显的 `window` / `metric` 一致

#### Scenario: Window 与 Metric 的多处切换同源
- **WHEN** 页面在顶部工具区与榜单区块两处都提供了 Window / Metric 切换，用户在其中一处切到 `week`
- **THEN** 另一处 MUST 同步显示为 `week`，MUST NOT 出现两处状态不一致
- **THEN** 页面 MUST 只按这一组参数发起一次请求

#### Scenario: 技术字面量不翻译
- **WHEN** 同一个页面分别在中文与英文界面下渲染
- **THEN** `successful_requests = actual_cost > 0` 这类字段名与运算式，以及 `today` / `week` / `month` 这类取值 MUST 在两种语言下完全相同
- **THEN** 区块标题、口径说明与卡片标签 MUST 分别渲染成中文与英文

#### Scenario: 页面恒深色且不改变站点主题
- **WHEN** 站点处于亮色主题（`html` 上没有 `dark` class）时渲染本页
- **THEN** 页面 MUST 渲染深色配色，根节点 MUST 只带承载 token 的那一个 class，MUST NOT 带任何明暗修饰 class
- **THEN** 页面在整个生命周期内 MUST NOT 增删 `html` 上的 `dark` class，也 MUST NOT 写 `localStorage` 里的 `theme`；离开本页后站点 MUST 仍是亮色

#### Scenario: 强调形态换了但判定不变
- **WHEN** 前两个条目的 `rank` 都是 `1`，另有一个条目的 `rank` 是 `4`
- **THEN** 前两行 MUST 都按第一名渲染，`rank` 为 `4` 的那行 MUST 按第四名渲染
- **THEN** 第一名的强调形态 MUST NOT 因为某行是第三行而被渲染

#### Scenario: 减少动效
- **WHEN** 查看者的系统设置是 `prefers-reduced-motion: reduce`
- **THEN** 页面 MUST 没有任何动画与数字滚动，也 MUST NOT 有任何常驻动效
- **THEN** 页面内容 MUST 照常完整渲染

### Requirement: 响应包含 Extremes、Viewer Stats 与五块新增 Insights
响应体 MUST 在既有字段之外满足三件事。其一，`highlights` MUST 增加 `extremes`，包含 `night_owl`、`rising`、`omnivore`、`talker`、`max_single`、`streak` 六项，每一项在无人满足条件时 MUST 为 `null`，MUST NOT 用零值对象顶替；`rising` MUST 只在 `window=today` 时可能有值，其余两个 Window（榜单窗口）下 MUST 为 `null`。每一项 MUST 与 Leaderboard Entry（榜单条目）用同一个结构化身份 `{kind, username?, avatar_url?}` 并额外带 `ordinal`，该用户不在下发的 `entries` 内时 `ordinal` MUST 为 `null`。其二，`insights` MUST 增加 `profiles`、`platforms_today`、`weekly_rhythm`、`composition_today`、`cache_trend_14` 五块，任一块的数据源缺失时该块 MUST 为 `null`，MUST NOT 用 0 填充，也 MUST NOT 让整个响应失败；`highlights.site` MUST 增加 `avg_tokens_per_request`（全站该 Window 的 Total Tokens（总 tokens）除以 Successful Requests（成功请求数）），全站成功请求数为 0 时该字段 MUST 缺席、MUST NOT 记成 0。其三，响应 MUST 增加顶层字段 `viewer`，包含 `rank_history`、`models`、`cache_hit_rate` 与 `avg_tokens_per_request`，全部是查看者**本人**的真实数据。`viewer` MUST NOT 为 `null`，也 MUST NOT 随 `status` 变化——它不出自 Snapshot（榜单快照）；查看者没有名次历史时 `rank_history` MUST 是空数组，本窗口零用量时 `models` MUST 是空数组且两个比率字段 MUST 缺席。`extremes`、新增的 Insights（洞察）与 `viewer` MUST NOT 使响应出现 `user_id`、邮箱或任何金额字段——Cost（消费金额）只出现在 Leaderboard Entry、`my_rank` 与 `highlights.site` 三处。

#### Scenario: rising 只在今日窗口存在
- **WHEN** 客户端请求 `window=week`
- **THEN** `highlights.extremes.rising` MUST 为 `null`
- **THEN** 页面 MUST 隐藏「进步之星」这张卡，MUST NOT 渲染一张空卡

#### Scenario: 之最的领先者不在榜内
- **WHEN** 某个 Window 的连续活跃之最在当前 Metric（排名指标）的 `entries` 里没有对应条目
- **THEN** `highlights.extremes.streak.ordinal` MUST 为 `null`
- **THEN** 页面 MUST 把该卡的身份渲染成「榜外用户」，MUST NOT 编一个序号出来

#### Scenario: 快照尚未生成时的三块数据
- **WHEN** 响应的 `status` 是 `computing`
- **THEN** `highlights` MUST 为 `null`，`extremes` 与 `insights.profiles` MUST 随之不可用
- **THEN** `viewer` MUST 照常返回，MUST NOT 为 `null`

#### Scenario: 周内节奏的来源缺行
- **WHEN** 站点关闭了仪表盘预聚合作业，近 28 天的小时桶一行都没有
- **THEN** `insights.weekly_rhythm` MUST 为 `null`
- **THEN** `insights.profiles`、`insights.platforms_today` 与 `insights.composition_today` MUST 照常返回

#### Scenario: 查看者本窗口零用量
- **WHEN** 查看者在当前 Window 内没有任何用量
- **THEN** `viewer.models` MUST 是空数组，`viewer.cache_hit_rate` 与 `viewer.avg_tokens_per_request` MUST 缺席
- **THEN** `viewer` 本身 MUST NOT 为 `null`，`viewer.rank_history` MUST 照常返回已有的历史点

#### Scenario: 新增字段不引入身份与金额
- **WHEN** 检查任意档位下的完整响应体
- **THEN** `extremes`、新增的五块 Insights 与 `viewer` 里 MUST NOT 出现 `user_id`、邮箱或金额
- **THEN** 它们里唯一可能出现的身份信息 MUST 只有 `identity.username`

### Requirement: Cost 的下发形态、提示语与站点合计
Cost（消费金额）MUST 以 USD 的浮点数下发，字段名在 Leaderboard Entry（榜单条目）与 `my_rank` 上都是 `cost`，在站点合计上是 `highlights.site.cost`。它 MUST 与 Total Tokens（总 tokens）走同一套档位规则：`named` 档与 Preview（预览）下发绝对金额；`anonymous` 档下他人条目的 `cost` MUST 缺席、由 `cost_relative_percent`（相对该 Window（榜单窗口）Cost 第一名的 0 到 100 整数百分比，第一名 MUST 是 `100`）代替，`highlights.site.cost` MUST 缺席；查看者本人的条目与 `my_rank.cost` MUST 在任何档位下都是真实金额。系统 MUST NOT 为 Cost 新增 Highlights（趣味卡）——`highlights` 里 MUST NOT 出现 `top_cost` 或等价的卡。金额在 Snapshot（榜单快照）内部以定点 micros（1 USD = 1e6）存放，响应 MUST 只下发换算回 USD 的值，MUST NOT 把 micros 泄露给客户端。「你的位置」的提示语 MUST 增加一种 kind `cost_to_top10`，其数值 MUST 是 USD 的金额差额（按 micros 算完再换算），tokens 与 requests 两种 kind 的 JSON 形态 MUST 不变。名次历史 MUST 一并记录按 Cost 的当日名次（见 `leaderboard-snapshot`），但页面上的名次走势折线 MUST 仍按 Total Tokens 的名次绘制，MUST NOT 因此新增一个折线的 Metric 切换。

#### Scenario: named 档下的金额列
- **WHEN** `named` 档下客户端请求 `metric=cost`
- **THEN** `entries` MUST 按 Cost 降序排列，每个条目 MUST 给出 `cost`（USD）
- **THEN** 每个条目 MUST 仍然给出 `total_tokens` 与 `successful_requests`，页面 MUST 高亮金额列

#### Scenario: anonymous 档下他人的金额
- **WHEN** `anonymous` 档下查看者查看榜单
- **THEN** 他人条目 MUST NOT 包含 `cost`
- **THEN** 他人条目 MUST 给出 `cost_relative_percent`，是 0 到 100 的整数，该 Window Cost 第一名的值 MUST 是 `100`

#### Scenario: 本人金额始终真实
- **WHEN** `anonymous` 档下查看者本人进入前 50
- **THEN** 其条目的 `cost` 与 `my_rank.cost` MUST 都是真实金额
- **THEN** 系统 MUST NOT 因为档位而把本人的金额换成相对百分比

#### Scenario: 还差多少进前 10
- **WHEN** `named` 档下查看者按 `metric=cost` 排在第 10 名之外
- **THEN** `my_rank.hint.kind` MUST 是 `cost_to_top10`，其数值 MUST 是进前 10 所差的金额（USD）
- **THEN** 页面 MUST 按查看者语言渲染成「再消费 $X 进前 10」一类的句子

#### Scenario: 不为金额新增趣味卡
- **WHEN** 检查任意档位下的完整响应体
- **THEN** `highlights` MUST NOT 包含 `top_cost` 或任何以金额为主角的新卡
- **THEN** `named` 档与 Preview 下 `highlights.site.cost` MUST 照常给出，供页面算「前三名占全站百分之多少」

### Requirement: 榜单身份的头像字段
Leaderboard 的结构化身份 MUST 增加一个可选字段 `avatar_url`，它与 `username` 受同一条规则约束：MUST 只在 `identity.kind` 为 `named` 时可能出现；`kind` 为 `anonymous` 或 `self` 的身份里 MUST NOT 出现 `avatar_url`。`avatar_url` 的值 MUST 是该用户头像的 64px 正方形小图（`user_avatars.thumb_url`，JPEG 的 data URL），系统 MUST NOT 在榜单响应里下发原图，也 MUST NOT 下发任何指向用户自填外链（`remote_url` 形态的头像）的地址——该类头像没有小图，其条目 MUST 直接省略 `avatar_url`，由前端回退成首字母圆圈。查看者本人的头像 MUST 由前端从本人的个人资料取，后端 MUST NOT 为 `self` 条目下发 `avatar_url`。头像 MUST NOT 有独立的用户开关：用户关闭 Named Participation（昵称展示）后，其 `username` 与 `avatar_url` MUST 一起消失。头像的下发 MUST NOT 影响响应的其它部分——取头像失败时系统 MUST 照常返回榜单，只是该批条目没有 `avatar_url`，MUST NOT 因此让请求失败。榜单条目、Highlights（趣味卡）、Extremes（之最）与模型偏好画像 MUST 复用同一个身份对象，MUST NOT 各自再判定一次。

#### Scenario: 实名条目带头像
- **WHEN** `named` 档下某实名用户上传过 inline 头像，其小图已就绪
- **THEN** 其条目的 `identity.avatar_url` MUST 是该头像的 64px 小图
- **THEN** 页面 MUST 在该行的 Display Name（展示名）前渲染这张头像

#### Scenario: 外链头像不上榜
- **WHEN** `named` 档下某实名用户的头像是自填的外链地址（没有小图）
- **THEN** 其条目 MUST NOT 包含 `avatar_url`
- **THEN** 页面 MUST 回退成首字母圆圈，MUST NOT 向该外链地址发起请求

#### Scenario: 本人行的头像不由后端下发
- **WHEN** 查看者本人出现在 `entries` 中
- **THEN** 该条目的 `identity` MUST NOT 包含 `avatar_url`
- **THEN** 页面 MUST 用查看者自己个人资料里的头像渲染该行

#### Scenario: 关掉昵称展示后头像一并消失
- **WHEN** 某用户把 Named Participation 关掉了
- **THEN** 其条目 MUST 是匿名形态，既 MUST NOT 包含 `username`，也 MUST NOT 包含 `avatar_url`
- **THEN** 系统 MUST NOT 为头像提供另一个独立开关

#### Scenario: 取头像失败不影响榜单
- **WHEN** 组装响应时批量读取头像小图出错
- **THEN** 响应 MUST 照常返回完整的 `entries`、`my_rank` 与 `participant_count`
- **THEN** 这一批条目 MUST 只是没有 `avatar_url`，系统 MUST NOT 因此返回错误

#### Scenario: Highlights 与模型画像复用同一个身份
- **WHEN** 某实名用户同时是 `highlights.top_tokens` 的领先者并出现在模型偏好画像里
- **THEN** 两处的 `identity.avatar_url` MUST 与其榜单条目上的值一致
- **THEN** 系统 MUST NOT 为这两处额外查询一次头像
