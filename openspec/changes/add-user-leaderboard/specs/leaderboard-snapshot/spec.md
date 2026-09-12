## Purpose

定义 Snapshot（榜单快照）的重建契约：后台周期作业与选主、一条 SQL 同时产出三个 Window（榜单窗口）的聚合、参与资格的过滤、Redis 上的派生结构与 key 命名及 TTL、原子切换、请求路径只读、陈旧上限，以及不设清空钩子的约束。Leaderboard（排行榜）向用户展示的行为见 `user-leaderboard`，档位差异见 `leaderboard-mode`。

## ADDED Requirements

### Requirement: Snapshot 是榜单三处数字的唯一来源
Snapshot MUST 是一个 Window 内所有有用量的合格用户的 Total Tokens（总 tokens）与 Successful Requests（成功请求数）集合，由后台周期性重建。Leaderboard Entry（榜单条目）、Participant Count（参与人数）与 My Rank（我的名次）MUST 全部从同一份 Snapshot 导出，任何时刻 MUST 互相自洽。Snapshot MUST 只记录 `user_id` 与数值（数值集合见「每用户 Hash 存四个数值」），MUST NOT 记录身份（`username`、邮箱、Display Name（展示名））或任何金额。

#### Scenario: 三处数字同源
- **WHEN** 查看者在同一次请求里拿到 `entries`、`participant_count` 与 `my_rank`
- **THEN** 三者 MUST 出自同一份 Snapshot
- **THEN** `my_rank.rank` MUST NOT 与 `entries` 中同一用户的 `rank` 冲突

#### Scenario: Snapshot 不含身份与金额
- **WHEN** 检查某个 Window 的 Snapshot 内容
- **THEN** 其中 MUST 只有 `user_id` 与数值字段
- **THEN** 其中 MUST NOT 出现 `username`、邮箱或任何金额

### Requirement: 后台周期作业每 5 分钟重建，leader lock 保证只跑一份
Snapshot 的重建 MUST 由一个经 `TimingWheelService.ScheduleRecurring` 注册的后台作业承担，周期为 5 分钟。每一轮 MUST 先通过 `tryAcquireSingletonLeaderLock` 取锁，保证多实例部署下同一轮只有一个实例真正执行聚合；Redis 不可用时 MUST 回落到 Postgres advisory lock。未取到锁的实例 MUST 跳过本轮，MUST NOT 自行聚合。

#### Scenario: 多实例部署
- **WHEN** 三个实例在同一轮周期同时到点
- **THEN** MUST 只有一个实例执行聚合 SQL
- **THEN** 另外两个实例 MUST 直接跳过本轮，MUST NOT 写入 Redis

#### Scenario: Redis 不可用
- **WHEN** 作业到点时 Redis 不可用
- **THEN** 选主 MUST 回落到 Postgres advisory lock
- **THEN** Redis 恢复后的下一轮 MUST 正常重建 Snapshot

#### Scenario: 请求不触发重建
- **WHEN** 大量用户在两轮作业之间访问 Leaderboard
- **THEN** 重建 MUST 只按 5 分钟周期发生
- **THEN** 请求 MUST NOT 触发任何一次聚合

### Requirement: 一条 SQL 同时产出三个窗口
每一轮重建 MUST 用一条 SQL、一次扫描 `usage_logs` 同时产出今日 / 本周 / 本月三个 Window 的按用户聚合：扫描下界 MUST 取 `min(月初, 周一)`，三个窗口各自用条件聚合（`FILTER (WHERE ul.created_at >= 该窗口起点)`）算出 Total Tokens 与 Successful Requests 两个数，`GROUP BY user_id`。窗口起点 MUST 用站点时区的 `StartOfDay` / `StartOfWeek`（周一起算）/ `StartOfMonth` 计算。Successful Requests MUST 沿用 `usageLogSuccessFilterUL`（`ul.actual_cost > 0`）的过滤约定。系统 MUST NOT 为每个窗口各跑一次聚合。

#### Scenario: 一轮只扫一次
- **WHEN** 后台作业执行一轮重建
- **THEN** MUST 只对 `usage_logs` 做一次扫描
- **THEN** 三个 Window 的六组数值 MUST 全部来自这一次扫描

#### Scenario: 扫描下界
- **WHEN** 站点时区当天是某月 3 日周三，本月起点早于本周周一
- **THEN** 扫描下界 MUST 取月初
- **THEN** 早于该下界的日志 MUST NOT 参与本轮聚合

#### Scenario: 今日窗口的条件聚合
- **WHEN** 某用户在本月有大量用量，但今日没有任何请求
- **THEN** 其今日窗口的两个数值 MUST 都是 0（即不进入今日 Snapshot）
- **THEN** 其本月窗口的数值 MUST 正常反映本月用量

### Requirement: 聚合排除禁用与软删除的用户
聚合 MUST 用 `INNER JOIN users` 并过滤 `users.status <> 'disabled' AND users.deleted_at IS NULL`，MUST NOT 使用 `LEFT JOIN`：不合格用户与孤儿日志都不应进入 Snapshot，也不应计入 Participant Count。管理员账号 MUST 照常参与聚合。

#### Scenario: 被禁用的用户
- **WHEN** 某用户的 `status` 是 `disabled` 但窗口内有用量
- **THEN** 其数据 MUST NOT 进入 Snapshot

#### Scenario: 已软删除的用户
- **WHEN** 某用户的 `deleted_at` 非空但窗口内有用量
- **THEN** 其数据 MUST NOT 进入 Snapshot

#### Scenario: 孤儿日志
- **WHEN** `usage_logs` 中存在 `user_id` 在 `users` 中已无对应行的记录
- **THEN** `INNER JOIN` MUST 把这些记录排除
- **THEN** 它们 MUST NOT 产生任何榜单条目

### Requirement: Redis 存派生结构而不是整块 JSON
Snapshot MUST 以派生结构写入 Redis：每个 Window × Metric 一个 ZSET（member = `user_id`，score = 该 Metric 的值），用于 Top 50（`ZREVRANGE`）、Participant Count（`ZCARD`）与 Rank（名次）（`ZCOUNT` 取严格高于自己的人数后加一）；每个 Window 另有一个 Hash 存 `user_id` → 该窗口的数值（字段集见「每用户 Hash 存四个数值」），供「每行同时显示两个 Metric」与 Cache Hit Rate（缓存命中率）使用；再存一个该 Window 的 Snapshot 更新时间；另有该 Window 的 Highlights（趣味卡）与站点级 Insights（洞察）两个 JSON 串（见「Highlights 与 Insights 在同一轮作业里算好」）。系统 MUST NOT 把整份榜单序列化成单个 JSON key。

#### Scenario: 取前 50 名
- **WHEN** 请求需要某 Window + Metric 的榜单条目
- **THEN** 系统 MUST 从对应 ZSET 按分数倒序取前 50 个 member
- **THEN** 系统 MUST NOT 读取并反序列化全体用户的数据

#### Scenario: 计算竞争名次
- **WHEN** 请求需要查看者的 Rank
- **THEN** 系统 MUST 用 `ZCOUNT` 统计分数严格高于查看者的成员数再加一
- **THEN** 与查看者同分的用户 MUST NOT 计入该统计

#### Scenario: 取另一个 Metric 的值
- **WHEN** 榜单按 `total_tokens` 排序但每行还要显示 Successful Requests
- **THEN** 系统 MUST 从该 Window 的 Hash 中批量读取这些 `user_id` 的数值

#### Scenario: 统计参与人数
- **WHEN** 请求需要 Participant Count
- **THEN** 系统 MUST 用 `ZCARD` 读取该 Window ZSET 的成员数

### Requirement: key 带窗口起点，TTL 到窗口结束
Redis key MUST 带上窗口起点（如 `leaderboard:v1:today:20260911`、`leaderboard:v1:week:20260907`、`leaderboard:v1:month:202609`），MUST NOT 只按窗口名命名。key 前缀 MUST 沿用 `cfg.Dashboard.KeyPrefix` 做环境隔离。每个 key 的 TTL MUST 取 `min(60 分钟, 距该窗口结束的时长)`，使跨零点、跨周一、跨月初时旧 key 自然作废。

#### Scenario: 跨零点
- **WHEN** 站点时区跨过零点进入新的一天，而新一轮作业尚未跑完
- **THEN** 请求 MUST 指向新一天起点对应的 key
- **THEN** 系统 MUST NOT 读到昨天残留的今日榜数据

#### Scenario: 跨周一
- **WHEN** 站点时区跨过周一零点进入新的一周，而新一轮作业尚未跑完
- **THEN** `window=week` 的请求 MUST 指向新周一起点对应的 key（如 `leaderboard:v1:week:20260914`）
- **THEN** 系统 MUST NOT 读到上一周残留的本周榜数据

#### Scenario: 跨月初
- **WHEN** 站点时区跨过月初零点进入新的一月，而新一轮作业尚未跑完
- **THEN** `window=month` 的请求 MUST 指向新月份起点对应的 key（如 `leaderboard:v1:month:202610`）
- **THEN** 系统 MUST NOT 读到上一月残留的本月榜数据，MUST 返回「正在计算」状态

#### Scenario: TTL 到窗口结束
- **WHEN** 某个今日窗口的 key 在当天下午被写入
- **THEN** 其 TTL MUST NOT 超过距当天窗口结束的时长
- **THEN** 该 key MUST 在窗口结束后自然过期

#### Scenario: 环境隔离
- **WHEN** 两个环境共用同一个 Redis 实例
- **THEN** 两边的 Leaderboard key MUST 因 `cfg.Dashboard.KeyPrefix` 不同而互不干扰

### Requirement: 先写临时 key 再 RENAME 原子切换
每一轮重建 MUST 用 pipeline 先把新数据写入临时 key，再通过 `RENAME` 切换到正式 key。读者 MUST NOT 在任何时刻读到半份榜单。某一轮作业在写入过程中失败时，正式 key MUST 保持上一轮的完整内容。

#### Scenario: 重建过程中的读者
- **WHEN** 某个请求恰好落在一轮重建的写入过程中
- **THEN** 该请求 MUST 读到上一轮完整的 Snapshot 或本轮完整的 Snapshot
- **THEN** 该请求 MUST NOT 读到只写了一半的 ZSET 或 Hash

#### Scenario: 本轮聚合失败
- **WHEN** 某一轮的聚合 SQL 或写入中途失败
- **THEN** 正式 key MUST 保持上一轮的内容不变
- **THEN** 该 Window 的 Snapshot 更新时间 MUST NOT 被推进

### Requirement: 请求路径只读 Snapshot，缺失时返回「正在计算」
HTTP 请求路径 MUST 只读 Redis 上的 Snapshot，MUST NOT 触发聚合、MUST NOT 回源查询 `usage_logs`。Snapshot 缺失（key 不存在或 Redis 不可用）时，系统 MUST 返回明确的「正在计算」状态，MUST NOT 返回空榜、也 MUST NOT 返回 500。请求路径上唯一允许的数据库访问 MUST 是为渲染展示名与参与资格而做的、至多 51 行（Top 50 + 查看者）的按 id 批量查询。

#### Scenario: 快照缺失
- **WHEN** 首次开启 Leaderboard Mode（排行榜模式）后第一轮作业还没写入任何 key
- **THEN** 响应 MUST 是「正在计算」状态
- **THEN** 系统 MUST NOT 因此执行聚合 SQL

#### Scenario: Redis 不可用
- **WHEN** 请求到达时 Redis 不可用
- **THEN** 系统 MUST 返回「正在计算」状态而不是 500
- **THEN** 系统 MUST NOT 回落到实时查询 `usage_logs`

#### Scenario: 并发请求下的数据库压力
- **WHEN** 大量用户同时访问 Leaderboard
- **THEN** 对 `usage_logs` 的查询次数 MUST 是 0
- **THEN** 对 `users` 的查询 MUST 每个请求至多一次、至多 51 行

### Requirement: Snapshot 超过 15 分钟未更新即为陈旧
响应 MUST 带上该 Window 的 Snapshot 更新时间。更新时间距当前时刻超过 15 分钟时，系统 MUST 标记为陈旧，响应 MUST 仍返回已有数据，但页面 MUST 显示警告而不是静默展示旧数据。

#### Scenario: 一个周期内的正常陈旧度
- **WHEN** Snapshot 更新时间在 6 分钟前
- **THEN** 响应 MUST NOT 带陈旧标记
- **THEN** 页面 MUST 正常展示更新时间

#### Scenario: 超过上限
- **WHEN** 后台作业因故停摆，Snapshot 更新时间已在 16 分钟前
- **THEN** 响应 MUST 带陈旧标记并仍返回数据
- **THEN** 页面 MUST 显示陈旧警告

### Requirement: 展示名与参与资格在响应时按当前状态渲染
数值与名次 MUST 随 Snapshot 冻结；Display Name 与参与资格 MUST 在响应时按 `users` 表的当前状态渲染，MUST NOT 冻结进 Snapshot。Snapshot 里仍存在但当前已不合格（`status = disabled` 或 `deleted_at` 非空）的用户，其条目 MUST 在渲染时被剔除，且 MUST NOT 下发其任何身份信息；被剔除后其余条目的 Rank MUST NOT 重算；Ordinal（行序号）MUST 在剔除完成后按剩余条目重新从 1 连续分配，以维持 `user-leaderboard` 对 Ordinal「唯一、不跳号」的定义。剔除后 MUST NOT 从第 51 名起回填，`entries` 的条数因此可以少于 50。`participant_count` MUST 取 Snapshot 的 `ZCARD`，渲染时剔除 MUST NOT 扣减它；偏差在下一轮周期重建后修正。

#### Scenario: 用户被禁用后快照未刷新
- **WHEN** 某用户在上一轮 Snapshot 中位列第 3，随后被禁用，Snapshot 中仍有其条目
- **THEN** 下一次响应 MUST 在渲染时剔除该条目，`entries` MUST NOT 包含它
- **THEN** 响应 MUST NOT 下发该用户的 `username` 或任何身份信息

#### Scenario: 剔除后 Rank 不变而 Ordinal 重排
- **WHEN** Snapshot 中 `ordinal` 为 1 / 2 / 3 的三个条目里，第 2 个的用户当前已被禁用而在渲染时被剔除
- **THEN** 剩余两个条目的 `rank` MUST 保持剔除前的值，MUST NOT 重算
- **THEN** 剩余两个条目的 `ordinal` MUST 是 `1` 与 `2`，MUST NOT 出现空缺

#### Scenario: 用户改了 username
- **WHEN** 某已实名展示的用户修改了 `username`
- **THEN** 下一次响应 MUST 使用新的 `username`
- **THEN** 系统 MUST NOT 等到下一轮 Snapshot 才生效

#### Scenario: 数值不随当前状态变化
- **WHEN** 某用户在两轮 Snapshot 之间产生了新的用量
- **THEN** 响应中的数值 MUST 仍是 Snapshot 冻结时的值
- **THEN** 页面 MUST 通过展示 Snapshot 更新时间说明这一点

### Requirement: 不因禁用、删除或切换模式而清空 Snapshot
Snapshot MUST NOT 因用户被禁用、被删除或 Leaderboard Mode 被切换而被清空或立即重建。下架与档位变化 MUST 由响应组装层即时体现：不合格用户在渲染时剔除，身份形态与数值精度按响应时刻的档位决定。Participant Count 与 Rank 的偏差 MUST 在下一轮周期重建后修正。理由：Snapshot 不含身份，切换模式不会泄露；而清空会让全站在下一轮重建前只看到「正在计算」。

#### Scenario: 紧急下架某个用户
- **WHEN** 管理员因投诉禁用了某个正在榜上的用户
- **THEN** 下一次响应 MUST 在渲染时剔除该用户，MUST NOT 下发其任何身份信息
- **THEN** 系统 MUST NOT 清空 Snapshot，其他用户 MUST 仍能看到榜单而不是「正在计算」

#### Scenario: 切换到更严的档位
- **WHEN** 管理员把 Leaderboard Mode 从 `named` 切到 `anonymous`
- **THEN** guard 缓存刷新后的响应 MUST 按 `anonymous` 档渲染，MUST NOT 出现 `username` 或他人的精确数值
- **THEN** 系统 MUST NOT 清空 Snapshot

#### Scenario: 批量禁用
- **WHEN** 管理员在一分钟内禁用了 100 个用户
- **THEN** 系统 MUST NOT 因此触发任何额外的聚合或清空
- **THEN** 这些用户 MUST 从后续响应的 `entries` 中消失，`participant_count` 在下一轮重建后修正

### Requirement: 每用户 Hash 存四个数值
本条已被「每用户 Hash 存十二段数值并向后兼容旧段数」取代：四段是 v1 的形态，v2 把 value 扩到十二段，前四段的顺序、口径与「缺段按 0」的兼容规则原样保留，因此下面的约束仍然逐条成立。每个 Window（榜单窗口）的 Hash MUST 至少存四个数值：`total_tokens`、`successful_requests`、`input_tokens`、`cache_read_tokens`。新增的两个数 MUST 由同一条聚合 SQL 在同一次扫描里按窗口条件聚合产出，MUST NOT 另起一次扫描。ZSET MUST 仍然只有两个 Metric（排名指标）——新增的两个数只用于计算 Cache Hit Rate（缓存命中率），MUST NOT 参与排名。Hash value 的解码 MUST 能容忍段数不足的旧值：缺失的段 MUST 按 0 处理，MUST NOT 报错、MUST NOT 使整个窗口的快照被判为无效。Hash 里 MUST 仍然只有 `user_id` 与数值，MUST NOT 出现身份或任何金额。

#### Scenario: 一次扫描出四个数
- **WHEN** 后台作业执行一轮重建
- **THEN** 每个用户每个窗口的四个数值 MUST 全部来自同一次 `usage_logs` 扫描
- **THEN** 系统 MUST NOT 为 `input_tokens` 与 `cache_read_tokens` 另跑一条查询

#### Scenario: 旧 Hash 缺字段时按 0 处理
- **WHEN** Redis 上还留着上一版写入的、只有两个数值的 Hash value
- **THEN** 系统 MUST 把缺失的 `input_tokens` 与 `cache_read_tokens` 按 0 处理并照常渲染榜单
- **THEN** 该窗口的 Cache Hit Rate MUST 表现为「没有命中率」而不是 0，并 MUST 在下一轮重建后自动补齐

#### Scenario: 新增的数不进 ZSET
- **WHEN** 检查某个 Window 的 Redis 结构
- **THEN** ZSET MUST 仍然只有 `total_tokens` 与 `successful_requests` 两个
- **THEN** 系统 MUST NOT 为 `input_tokens` 或 `cache_read_tokens` 建立排序结构

#### Scenario: 命中率无法计算
- **WHEN** 某用户该窗口的 `input_tokens + cache_read_tokens` 为 0
- **THEN** 该用户该窗口 MUST 没有 Cache Hit Rate
- **THEN** 系统 MUST NOT 把它记成 0

### Requirement: Highlights 与 Insights 在同一轮作业里算好
Highlights（趣味卡）与 Insights（洞察）MUST 与 Snapshot（榜单快照）在同一轮 5 分钟作业里算好并写入 Redis，请求路径 MUST NOT 为它们触发任何聚合或回源查询。每个 Window 的 Highlights MUST 存成一个 JSON 串，key MUST 带该 Window 的窗口起点，MUST 与该 Window 的 ZSET、Hash 同一轮写入、同样先写临时 key 再 `RENAME`、同一个 TTL。站点级 Insights MUST 存成一个与 Window 无关的 JSON 串，key MUST 带今日窗口起点以便跨零点自然作废，TTL MUST 与其它 key 同一条规则。效率之星要遍历该窗口全部用户，MUST 只在重建时做一次；其 dominant model 需要一条按用户与模型的聚合，MUST 只对选中的那一个用户查一次。Highlights MUST 只记录 `user_id` 与数值，MUST NOT 记录身份或任何金额——身份仍在响应时按 `users` 表当前状态渲染。

#### Scenario: 请求不触发 Highlights 计算
- **WHEN** 大量用户在两轮作业之间访问 Leaderboard（排行榜）
- **THEN** 效率之星的遍历与 dominant model 的聚合 MUST 都不发生
- **THEN** 请求路径对 `usage_logs` 的查询次数 MUST 仍然是 0

#### Scenario: Highlights 与榜单原子切换
- **WHEN** 某个请求恰好落在一轮重建的写入过程中
- **THEN** 该请求读到的 Highlights 与榜单 MUST 出自同一轮快照
- **THEN** 该请求 MUST NOT 读到只写了一半的 Highlights

#### Scenario: 跨零点的 Insights key
- **WHEN** 站点时区跨过零点进入新的一天，而新一轮作业尚未跑完
- **THEN** 请求 MUST 指向新一天起点对应的 Insights key
- **THEN** 系统 MUST NOT 读到昨天残留的今日模型热度、时段分布与缓存命中

#### Scenario: Highlights 不含身份
- **WHEN** 检查 Redis 上存着的 Highlights 内容
- **THEN** 其中 MUST 只有 `user_id` 与数值
- **THEN** 其中 MUST NOT 出现 `username`、邮箱或任何金额

#### Scenario: 本轮失败不推进
- **WHEN** 某一轮的 Highlights 或 Insights 计算中途失败
- **THEN** 正式 key MUST 保持上一轮的内容不变
- **THEN** 该 Window 的 Snapshot 更新时间 MUST NOT 被推进

### Requirement: 预聚合缺行时对应 Insights 区块返回 null
来自仪表盘预聚合表的 Insights 区块，在预聚合作业未启用或保留期未覆盖该时间段而缺行时，MUST 整块返回 `null`，MUST NOT 用 0 填充，MUST NOT 让整个响应失败，也 MUST NOT 回落到实时扫描 `usage_logs`。不依赖预聚合表的区块 MUST 照常返回。预聚合表的请求数是不带成功落账过滤的裸计数，与榜单的 Successful Requests（成功请求数）不是同一个口径，两者 MUST 用不同的字段名表达，页面 MUST 注明这一差异。

#### Scenario: 预聚合作业未启用
- **WHEN** 站点关闭了仪表盘预聚合作业，两张预聚合表都没有数据
- **THEN** 依赖它们的 Insights 区块 MUST 全部为 `null`
- **THEN** 系统 MUST NOT 因此回落到实时扫描 `usage_logs`

#### Scenario: 保留期没覆盖到近 30 天
- **WHEN** 预聚合表的保留窗口短于 30 天
- **THEN** 近 30 天区块 MUST 只返回有行的那些天，缺的天 MUST NOT 被补成 0
- **THEN** 其余区块 MUST 照常返回

#### Scenario: 不依赖预聚合的区块照常返回
- **WHEN** 预聚合表缺行而今日的 `usage_logs` 正常
- **THEN** 今日模型热度 MUST 照常返回
- **THEN** 榜单本体与 Highlights MUST 照常返回

#### Scenario: 两种请求数口径分开命名
- **WHEN** 同一份响应里既有来自预聚合表的请求数，又有来自榜单口径的请求数
- **THEN** 两者 MUST 用不同的字段名，MUST NOT 共用一个名字
- **THEN** 页面 MUST 注明预聚合口径包含失败请求的占位记录

### Requirement: 每用户 Hash 存十二段数值并向后兼容旧段数
每个 Window（榜单窗口）的 Hash value MUST 是十二段逗号分隔的数值，顺序固定为 `total_tokens`、`successful_requests`、`input_tokens`、`cache_read_tokens`、`output_tokens`、`cache_creation_tokens`、`night_tokens`、`distinct_models`、`max_single_tokens`、`media_requests`、`yesterday_tokens`、`yesterday_requests`。新增的八个数 MUST 由同一条聚合 SQL 在同一次扫描里产出，MUST NOT 另起扫描；扫描下界 MUST 相应从 `min(月初, 周一)` 放宽到 `min(月初, 周一, 昨日起点)`。解码 MUST 容忍段数不足的旧值：缺失的段 MUST 按 0 处理，MUST NOT 报错、MUST NOT 使整个窗口的快照被判为无效，两段式与四段式的历史 value MUST 都仍能读出榜单本体。ZSET MUST 仍然只有两个 Metric（排名指标），新增的八个数 MUST NOT 参与排名。`media_requests`（有图片或视频计数的成功请求数）MUST 只写入 Hash，MUST NOT 出现在任何响应字段里。Hash 里 MUST 仍然只有 `user_id` 与数值，MUST NOT 出现身份或任何金额。

#### Scenario: 一次扫描出十二个数
- **WHEN** 后台作业执行一轮重建
- **THEN** 每个用户每个窗口的十二段数值 MUST 全部来自同一次 `usage_logs` 扫描
- **THEN** 系统 MUST NOT 为新增的任何一段另跑一条查询

#### Scenario: 读到旧的四段式 value
- **WHEN** Redis 上还留着上一版写入的、只有四段的 Hash value
- **THEN** 系统 MUST 把缺失的八段按 0 处理并照常渲染榜单
- **THEN** 依赖这些段的之最卡 MUST 表现为缺席而不是 0，并 MUST 在下一轮重建后自动补齐

#### Scenario: 新增的数不进 ZSET
- **WHEN** 检查某个 Window 的 Redis 结构
- **THEN** ZSET MUST 仍然只有 `total_tokens` 与 `successful_requests` 两个
- **THEN** 系统 MUST NOT 为 `output_tokens`、`night_tokens` 或其余任何新增段建立排序结构

#### Scenario: media_requests 只存不发
- **WHEN** 检查任意档位下的完整响应体
- **THEN** 响应 MUST NOT 包含 `media_requests` 或任何等价字段
- **THEN** 该数值 MUST 仍然按位写进 Hash，以便后续增量直接可用

### Requirement: Extremes 与新增 Insights 在同一轮作业里算好
Extremes（之最）与新增的五块 Insights（洞察）MUST 与 Snapshot（榜单快照）在同一轮 5 分钟作业里算好并写入 Redis，请求路径 MUST NOT 为它们触发任何聚合或回源查询。按 Window（榜单窗口）计算的两块——`extremes` 与 `profiles`——MUST 写进该 Window 的 Highlights（趣味卡）JSON，与该 Window 的 ZSET、Hash 同一轮写入、同样先写临时 key 再 `RENAME`、同一个 TTL，因此一次请求读到的榜单、Highlights、Extremes 与 `profiles` MUST 必然同源。与 Window 无关的三块——`platforms_today`、`weekly_rhythm`、`cache_trend_14`——MUST 写进站点级 Insights 的 JSON；`composition_today` MUST 由今日窗口聚合的站点合计导出，MUST NOT 另查一次。`streak` MUST 读 `usage_dashboard_daily_users` 并回溯 90 天，MUST NOT 回去扫 `usage_logs`；它与 Window 无关，三个窗口的结果 MUST 相同。`profiles` 的按用户与模型聚合 MUST 只对该 Window 的前 8 名一次查出，MUST NOT 逐人各查一次。这两块 JSON MUST 只记录 `user_id` 与数值，MUST NOT 记录身份或任何金额——身份仍在响应时按 `users` 表当前状态渲染。

#### Scenario: 请求不触发之最的计算
- **WHEN** 大量用户在两轮作业之间访问 Leaderboard（排行榜）
- **THEN** 连续活跃的回溯查询、模型偏好画像的聚合与平台分布的聚合 MUST 都不发生
- **THEN** 请求路径对 `usage_logs` 的查询次数 MUST 仍然只有「你的统计」那一条例外

#### Scenario: 之最与榜单原子切换
- **WHEN** 某个请求恰好落在一轮重建的写入过程中
- **THEN** 该请求读到的 `extremes`、`profiles` 与榜单 MUST 出自同一轮快照
- **THEN** 该请求 MUST NOT 读到只写了一半的 Highlights

#### Scenario: 连续活跃不回扫日志表
- **WHEN** 作业计算连续活跃之最
- **THEN** 系统 MUST 只查 `usage_dashboard_daily_users` 的近 90 天切片
- **THEN** 三个 Window 的这一项 MUST 得到相同的结果

#### Scenario: 模型偏好画像只查一次
- **WHEN** 作业为某个 Window 计算前 8 名各自的 Top 3 模型
- **THEN** 系统 MUST 用一条限定这 8 个 `user_id` 的聚合查出全部结果
- **THEN** 系统 MUST NOT 为这 8 个用户各发一条查询

#### Scenario: 新增区块失败只降级本块
- **WHEN** 某一轮里周内节奏或缓存命中率趋势的来源读取失败
- **THEN** 对应区块 MUST 降级为 `null` 并记一条日志
- **THEN** 本轮的三个 Window 快照 MUST 照常写入，正式 key MUST NOT 停留在上一轮

### Requirement: 名次历史每轮 upsert 并按保留期清理
系统 SHALL 新增一张按「用户 × 日期」记录名次的表，主键 MUST 是 `(user_id, snapshot_date)`，每行 MUST 同时记录该日两个 Metric（排名指标）的名次。快照作业每一轮 MUST 对今日窗口的**全部参与者**做一次 upsert（`INSERT ... ON CONFLICT DO UPDATE`，分批写入），同一天的多轮 MUST 互相覆盖而不是追加，日终那一轮写下的 MUST 即该日的最终名次。同一轮 MUST 顺带删除保留期（90 天）之前的行。名次历史 MUST 只记录 `user_id`、日期与名次，MUST NOT 记录身份、数值或任何金额。响应里的名次走势 MUST 只取查看者本人近 14 天的行，MUST NOT 下发其他用户的任何名次历史。写入或清理失败 MUST NOT 让整轮快照重建失败，也 MUST NOT 阻断响应——该块降级为空数组。

#### Scenario: 同一天多轮覆盖
- **WHEN** 某一天里作业跑了若干轮，某用户的名次从 18 变成 12
- **THEN** 该用户当天 MUST 只有一行，`rank_total_tokens` MUST 是最近一轮写下的 12
- **THEN** 系统 MUST NOT 为同一天同一用户追加多行

#### Scenario: 保留期清理
- **WHEN** 作业执行一轮重建，表里存在 91 天前的行
- **THEN** 这些行 MUST 在本轮被删除
- **THEN** 90 天以内的行 MUST 全部保留

#### Scenario: 只下发本人的名次历史
- **WHEN** 查看者请求 Leaderboard（排行榜）
- **THEN** `viewer.rank_history` MUST 只包含查看者本人近 14 天的 `{date, rank}`
- **THEN** 响应 MUST NOT 包含任何其他用户的名次历史

#### Scenario: 名次历史写入失败
- **WHEN** 某一轮的名次历史 upsert 失败
- **THEN** 本轮的三个 Window 快照 MUST 照常写入
- **THEN** 系统 MUST 记一条日志，MUST NOT 因此让整轮重建中止

#### Scenario: 新用户还没有历史
- **WHEN** 查看者是第一次产生用量、表里还没有他的行
- **THEN** `viewer.rank_history` MUST 是空数组
- **THEN** 页面 MUST 隐藏名次走势那一格，MUST NOT 画一条空折线

### Requirement: viewer.models 是请求路径唯一的回源例外，结果缓存 60 秒
`viewer.models`（查看者本人在当前 Window（榜单窗口）的 Top 5 模型及占比）MUST 是「请求路径只读 Snapshot（榜单快照）」的唯一例外：系统 MAY 为它执行一条 `usage_logs` 聚合，该查询 MUST 带 `user_id = 查看者` 且限定在该 Window 的时间区间内，MUST NOT 扫描其他用户的行，也 MUST NOT 用于任何其他字段。查询结果 MUST 在 Redis 上按 `(user_id, window, 窗口起点)` 缓存，TTL MUST 是 60 秒量级且 MUST NOT 超过距该窗口结束的时长。缓存命中时 MUST NOT 再查一次数据库。Redis 不可用或查询失败时该块 MUST 降级为空数组，MUST NOT 返回 500，也 MUST NOT 影响响应的其余部分。除这一条之外，请求路径对 `usage_logs` 的查询次数 MUST 仍然是 0。

#### Scenario: 只查自己
- **WHEN** 系统为某个查看者计算 `viewer.models`
- **THEN** 该聚合 MUST 带 `user_id = 查看者` 的过滤
- **THEN** 结果 MUST NOT 包含任何其他用户的模型用量

#### Scenario: 60 秒内复用缓存
- **WHEN** 同一个查看者在 60 秒内连续多次请求同一个 Window
- **THEN** 只有第一次 MUST 触达数据库
- **THEN** 其余请求 MUST 读缓存，对 `usage_logs` 的查询次数 MUST 是 0

#### Scenario: 跨零点不串味
- **WHEN** 站点时区跨过零点进入新的一天
- **THEN** `window=today` 的 `viewer.models` 缓存 MUST 指向新一天起点对应的 key
- **THEN** 系统 MUST NOT 读到昨天残留的结果

#### Scenario: 例外不外溢
- **WHEN** 检查一次完整请求对数据库的访问
- **THEN** 对 `usage_logs` 的查询 MUST 至多只有 `viewer.models` 这一条
- **THEN** 榜单本体、Highlights（趣味卡）、Extremes（之最）与其余 Insights（洞察）MUST 全部只读 Redis

#### Scenario: 查询失败时降级
- **WHEN** 该聚合查询超时或 Redis 不可用
- **THEN** `viewer.models` MUST 是空数组
- **THEN** 响应的其余部分 MUST 照常返回，系统 MUST NOT 返回 500
