# sub2api

sub2api 的领域词汇表。目前只覆盖「用量排行」相关概念，随设计推进逐步补充。本文件只是词汇表，不记录实现决策。

## Language

### 用量排行

**Leaderboard（排行榜）**:
面向登录用户的、按固定窗口聚合的用户用量名次表，只暴露受限字段（展示名、头像、tokens、请求数、消费金额），不暴露邮箱和用户 id。模式开启后，管理员与普通用户看到相同数据。与渠道监控里的用户排行是两个不同的功能：监控是诊断视图，Leaderboard 是社交视图。
_Avoid_: 用户排行、token ranking、top users

**User Breakdown（用户用量明细）**:
面向管理员的按用户聚合的用量分析视图，支持任意起止日期，包含全部字段（含邮箱与三种金额口径）。与 Leaderboard 是两个不同的概念，不共用名字。
_Avoid_: 用户排行、tokenRanking

**Window（榜单窗口）**:
Leaderboard 聚合所依据的固定时间段之一：今日、本周、本月。边界一律按站点时区计算，周从周一开始；用户不能自定义起止日期，也没有「全部时间」窗口。
_Avoid_: 日期范围、date range、period、全部

**Metric（排名指标）**:
Leaderboard 排序所依据的量：Total Tokens、Successful Requests 或 Cost。只有这三项，默认 Total Tokens；每一行同时给出三个值，按选中的那个排序。
_Avoid_: sort key、sort_by

**Total Tokens（总 tokens）**:
一次请求的 input、output、cache_creation、cache_read 四类 tokens 之和。与管理端用户用量明细和用户仪表盘的口径一致。
_Avoid_: tokens、token 总数（不写明是否含 cache）

**Successful Requests（成功请求数）**:
成功落账（actual_cost 大于 0）的请求数。失败请求的占位记录不计入。与管理端用户用量明细的「请求数」（不做过滤的计数）是两个口径。
_Avoid_: 请求数、requests、调用次数

**Cost（消费金额）**:
一个 Window 内实际计费金额（`actual_cost`）之和，单位 USD。与用户自己的用量页「花费」同一个口径，与成功落账（`actual_cost > 0`）同源；不是账面成本 `total_cost`，也不是缓存省下的钱。隐私档位与 tokens 完全一致：实名模式给绝对金额，匿名模式下他人只给相对第一名的百分比，本人始终是真实值。
_Avoid_: 费用、成本、total_cost、消费额

**Leaderboard Entry（榜单条目）**:
Leaderboard 中的一行：名次、展示名、各 Metric 的值。至多 50 条。
_Avoid_: 行、item、row

**Rank（名次）**:
某用户在一个 Window + Metric 下的位置，等于该 Metric 值严格高于他的用户数加一。值相同的用户名次相同，其后的名次跳号（1、1、3）。榜单条目与 My Rank 都用这一条规则。
_Avoid_: 排名、position、序号

**Ordinal（行序号）**:
榜单条目在当前 Window + Metric 下的连续序号，从 1 起、唯一、不跳号。只用于匿名形态的展示名，不是 Rank。
_Avoid_: 名次、index、下标

**Display Name（展示名）**:
Leaderboard 上向查看者展示的身份。实名形态：username。匿名形态：「第 Ordinal 位」。本人行不受档位约束——有 username 就显示本人的 username（另带「你」标记），username 缺席或全空白时才显示「当前用户」；匿名档只约束他人的展示形态。他人只有在 Leaderboard Mode 为 named、该用户的 Named Participation 为开、且 username 合格（非空、非邮箱形态、通过校验）时才是实名形态，否则一律匿名形态。他人的展示名永远不是邮箱，也永远不是用户 id；本人行是前端拿自己的资料渲染的，不走这条校验，所以自己的 username 是什么就显示什么。头像（Avatar）与展示名同进同出：实名形态才可能有，匿名形态一律没有。
_Avoid_: 用户名、昵称、email、User #id、用户 #N、Me

**Avatar（头像）**:
Leaderboard 上跟在名次之后、展示名之前的 24px 圆形图像。只有实名形态才可能有：匿名形态是一个不带字符的素色圆圈，本人行显示本人个人资料里的头像（前端自己取，不经榜单响应下发）。榜单上走的是头像的小图（64px 正方形），不是原图；用户自填的外链头像没有小图，榜单不显示它，回退成首字母圆圈。它没有独立开关——关掉 Named Participation，名字和头像一起消失。
_Avoid_: 头像图、avatar url、profile picture、用户图片

**Named Participation（昵称展示）**:
用户在个人资料中的开关，**默认开启**（迁移 240 把既有用户一并回填为开）。开启表示 Leaderboard 以该用户的 username 与头像显示他；关闭时该用户仍参与排名，只是以匿名形态出现（名字与头像一起消失）。个人资料上的文案是「在排行榜显示我的昵称」——代码标识符与设计文档里出现的旧中文名「实名参与」指的是同一个开关，只是默认值与措辞已改。
_Avoid_: 上榜、退出排行、opt-in

**My Rank（我的名次）**:
当前查看者在某个 Window + Metric 下的 Rank，即使不在榜单条目内也会展示。查看者在该 Window 内没有任何用量时没有 My Rank，只提示「暂无用量」。本人行始终显示真实数值。
_Avoid_: 自己的排名、current user rank

**Participant Count（参与人数）**:
一个 Window 内有过任何用量的合格用户总数，作为 My Rank 的分母展示。anonymous 模式下只显示分档（如「100+ 人」），named 模式下显示精确值。
_Avoid_: 总人数、user count

**Leaderboard Mode（排行榜模式）**:
管理员控制的系统级设置，三档暴露递增，默认 off。off：Leaderboard 对普通用户既不可见也不可访问，管理员可 Preview。anonymous：所有人匿名形态，其他人的数值只显示相对第一名的百分比，Participant Count 分档。named：开启 Named Participation 的用户实名形态，所有数值精确，Participant Count 精确。设置无法读取时按 off 处理。
_Avoid_: 排行榜开关、功能开关、feature flag、enabled

**Preview（预览）**:
Leaderboard Mode 为 off 时管理员看到的 Leaderboard 视图，带明确的「普通用户不可见」标记。
_Avoid_: 管理员视图、admin view

**Snapshot（榜单快照）**:
一个 Window 内所有有用量的合格用户的 Total Tokens、Successful Requests 与 Cost 集合，由后台周期性重建。榜单条目、Participant Count、My Rank 都从同一份 Snapshot 导出，因此彼此一致；Snapshot 只记录用户与数值，不记录身份（用户名、邮箱、展示名、头像都不进去），身份与参与资格以展示时的当前状态为准。页面展示 Snapshot 的更新时间。
_Avoid_: 缓存、cache、聚合结果

**Highlights（趣味卡）**:
Leaderboard 顶部的四张卡：该 Window 的 Total Tokens 第一、Cache Hit Rate 第一、Successful Requests 第一，以及全站概况。标签随 Window 变化。身份与数值遵守与榜单条目相同的档位规则：anonymous 模式下领先者用 Ordinal 假名，不在榜单条目内时显示「榜外用户」。
_Avoid_: 卡片、榜单卡、highlight cards、明星榜

**Insights（洞察）**:
Leaderboard 页面上榜单以外的统计：今日模型热度、近 30 天活跃度、用量趋势、今日时段分布、今日缓存命中、本月累计、平台分布、周内节奏、Token 构成、缓存命中率趋势。这些块与 Window 无关，永远是站点口径，不是某个人的数据。模型偏好画像（profiles）是唯一的例外：它按 Window 计算、逐用户给出，只因页面区块归属才挂在 insights 下。数据源缺失时整块不展示，不是显示 0。
_Avoid_: 图表、统计、分析、dashboard

**Cache Hit Rate（缓存命中率）**:
缓存读取 tokens 占「输入 tokens 加缓存读取 tokens」的比例，按 Window 聚合，可以是某个用户的，也可以是全站的。缓存的收益一律用它和命中 tokens 表达，不折算成金额——页面上的金额只有 Cost 这一个口径，「省下多少钱」是另一回事。输入与缓存读取都为 0 时没有命中率，不是 0%。
_Avoid_: 命中率、缓存率、cache rate、省下的钱

**Share Percent（占比）**:
某个对象占全站同一口径总量的百分比，取整数：某用户占全站 Total Tokens 的比例，或某个模型占全站请求的比例。它是相对量，两个模式下都展示。与 Rank 无关，也不是「相对第一名的百分比」那个数。
_Avoid_: 百分比、占比率、percentage、相对第一名

**Extremes（之最）**:
Leaderboard 上四张 Highlights 之下的一排六张卡：夜猫子、进步之星、杂食者、话痨、单次最大、连续活跃。每张各取一人，按当前 Window 计算，身份与数值遵守与榜单条目相同的档位规则。进步之星只在今日窗口存在，连续活跃与 Window 无关。与 Highlights 是两类卡：Highlights 讲「谁最多」，Extremes 讲「谁最特别」。
_Avoid_: 成就、徽章、badge、achievements、之最榜

**Viewer Stats（你的统计）**:
Leaderboard 上只给查看者本人看的一块：名次走势、本人模型偏好、本人缓存命中率与平均每请求 tokens，以及与全站同口径值的对比。里面全是本人数据，因此在任何模式下都是真实值，不做档位裁剪。与「你的位置」是两回事：那一条讲名次与分母，这一块讲本人的用量画像。
_Avoid_: 个人中心、我的数据、personal dashboard、用户详情

**Rank History（名次走势）**:
某个用户逐日的名次记录，每天一个点。逐日留痕时三个 Metric 的名次都记，页面只画按今日窗口 Total Tokens 的那一条。当天之内随快照反复覆盖，日终那次即该日的最终名次；只保留近期，过期自动清理。页面只画查看者本人近两周的走势，永远不展示他人的名次历史。
_Avoid_: 排名历史、历史排行、rank trend、历史榜单

**Weekly Rhythm（周内节奏）**:
全站请求量在「周几 × 小时」这张 7 × 24 网格上的分布，取近四周的平均，只表达为相对最忙格子的等级，不给请求数本身。它是站点口径，不是某个人的作息。与今日时段分布是两块：那块是今天 24 小时的实况，这块是跨周的规律。
_Avoid_: 周报、活跃度、weekly report、作息

**Token Composition（Token 构成）**:
今日全站的 tokens 按输入、输出、缓存创建、缓存读取四类拆开的占比。占比在两个模式下都展示，绝对量只在实名模式下展示。它回答「tokens 花在哪儿」，与 Cache Hit Rate 回答的「缓存省了多少」是两个问题。
_Avoid_: token 分布、用量构成、token breakdown、成本构成

**Chapter（章）**:
Leaderboard 页面上带固定编号的内容区块，01 到 07 各自对应一组口径：今日高亮、六个之最、我的位置、榜单、模型与平台、活跃节奏、趋势与构成。编号是这块内容的身份而不是它的出场次序——某一章因数据缺失整章不渲染时，其余章的编号不重排，页面上出现跳号是正常的。
_Avoid_: 区块、板块、section、卡片组、第 N 块

