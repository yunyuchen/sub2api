import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbRankList from '../LbRankList.vue'
import type {
  LeaderboardEntry,
  LeaderboardMetric,
  LeaderboardSiteSummary,
} from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.table.rank': 'Rank',
  'leaderboard.table.user': 'User',
  'leaderboard.table.relativeToTop': 'Relative to #1',
  'leaderboard.table.totalTokens': 'Total tokens',
  'leaderboard.table.successfulRequestsShort': 'Successful',
  'leaderboard.table.cost': 'Spend',
  'leaderboard.table.relativePercent': '{percent}% of the top entry',
  'leaderboard.table.relativeUnknown': 'The top entry usage is not public in anonymous mode',
  'leaderboard.table.selfBadge': 'You',
  'leaderboard.identity.anonymous': 'Row {ordinal}',
  'channelMonitorV2.currentUser': 'Current user',
  'leaderboard.metrics.totalTokens': 'Total tokens',
  'leaderboard.metrics.successfulRequests': 'Successful requests',
  'leaderboard.metrics.cost': 'Spend',
  'leaderboard.rank.readout.lead': 'LEAD {ratio}x',
  'leaderboard.rank.readout.topThreeShare': 'SHARE {percent}%',
}

function translate(key: string, params?: Record<string, unknown>): string {
  const template = messages[key] ?? key
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? ''))
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: translate }) }
})

/**
 * 展示名 helper（`displayName.ts`）会读 auth store 里的 username：本人行优先显示自己的
 * 昵称，没有昵称时才回退「当前用户」。默认置空以覆盖回退分支，需要昵称的用例自己赋值。
 */
const authState = vi.hoisted(() => ({
  user: null as { username?: string; avatar_url?: string } | null,
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authState }))

beforeEach(() => {
  authState.user = null
})

function entry(overrides: Partial<LeaderboardEntry> = {}): LeaderboardEntry {
  return {
    rank: 1,
    ordinal: 1,
    identity: { kind: 'named', username: 'alice' },
    is_self: false,
    total_tokens: 4_200_000,
    successful_requests: 1203,
    cost: 12.34,
    ...overrides,
  }
}

interface MountOptions {
  metric?: LeaderboardMetric
  site?: LeaderboardSiteSummary | null
}

function mountList(entries: LeaderboardEntry[], options: MountOptions = {}) {
  return mount(LbRankList, {
    props: {
      entries,
      metric: options.metric ?? 'total_tokens',
      site: options.site ?? null,
    },
  })
}

/** 五行的实名档榜：#4 与 #5 在 tokens 上相邻，但请求数是反过来的（用来验互换分句）。 */
function namedBoard(): LeaderboardEntry[] {
  return [
    entry({ rank: 1, ordinal: 1, total_tokens: 4_000_000, successful_requests: 100, cost: 40 }),
    entry({
      rank: 2,
      ordinal: 2,
      identity: { kind: 'named', username: 'bob' },
      total_tokens: 2_000_000,
      successful_requests: 50,
      cost: 20,
    }),
    entry({
      rank: 3,
      ordinal: 3,
      identity: { kind: 'named', username: 'carol' },
      total_tokens: 1_000_000,
      successful_requests: 40,
      cost: 10,
    }),
    entry({
      rank: 4,
      ordinal: 4,
      identity: { kind: 'named', username: 'dave' },
      total_tokens: 100_000,
      successful_requests: 10,
      cost: 1,
    }),
    entry({
      rank: 5,
      ordinal: 5,
      identity: { kind: 'named', username: 'erin' },
      total_tokens: 90_000,
      successful_requests: 12,
      cost: 0.9,
    }),
  ]
}

function site(overrides: Partial<LeaderboardSiteSummary> = {}): LeaderboardSiteSummary {
  return {
    total_tokens: 10_000_000,
    successful_requests: 400,
    cost: 100,
    participant_count: 5,
    ...overrides,
  }
}

describe('LbRankList', () => {
  // 前三名的强调形态按 Rank 值判定：并列同名次一起算，跳号之后的名次不算。
  it('marks every row whose rank is 1–3, not the first three rows', () => {
    const wrapper = mountList([
      entry({ rank: 1, ordinal: 1 }),
      entry({ rank: 1, ordinal: 2, identity: { kind: 'named', username: 'erin' } }),
      entry({ rank: 3, ordinal: 3, identity: { kind: 'named', username: 'dave' } }),
      entry({ rank: 4, ordinal: 4, identity: { kind: 'named', username: 'grace' } }),
    ])

    const rows = wrapper.findAll('[data-testid="leaderboard-row"]')
    expect(rows[0].classes()).toContain('is-top')
    expect(rows[1].classes()).toContain('is-top')
    expect(rows[2].classes()).toContain('is-top')
    expect(rows[3].classes()).not.toContain('is-top')
    // 换皮后没有奖牌形态，也没有 emoji
    expect(wrapper.text()).not.toContain('🥇')
    expect(wrapper.text()).not.toContain('[1]')
  })

  it('zero-pads the rank column from the rank value, not the row index', () => {
    const wrapper = mountList([
      entry({ rank: 1, ordinal: 1 }),
      entry({ rank: 1, ordinal: 2 }),
      // 并列之后跳号：第三行的 Ordinal 是 3，名次却是 12 —— 数字跟 Rank 走
      entry({ rank: 12, ordinal: 3 }),
    ])

    const ranks = wrapper.findAll('.rp-lb-rank')
    expect(ranks[0].text()).toBe('01')
    expect(ranks[1].text()).toBe('01')
    expect(ranks[2].text()).toBe('12')
    expect(wrapper.findAll('[data-testid="leaderboard-rank-top"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="leaderboard-rank-plain"]')).toHaveLength(1)
  })

  it('highlights the viewer row with the self badge and keeps its absolute values', () => {
    const wrapper = mountList([
      entry({
        rank: 1,
        ordinal: 1,
        identity: { kind: 'anonymous' },
        total_tokens: undefined,
        successful_requests: undefined,
        cost: undefined,
        total_tokens_relative_percent: 100,
        successful_requests_relative_percent: 100,
        cost_relative_percent: 100,
      }),
      entry({
        rank: 2,
        ordinal: 2,
        identity: { kind: 'self' },
        is_self: true,
        total_tokens: 640_000,
        successful_requests: 233,
      }),
    ])

    const selfRow = wrapper.find('[data-testid="leaderboard-row-self"]')
    expect(selfRow.exists()).toBe(true)
    expect(selfRow.classes()).toContain('is-self')
    expect(selfRow.text()).toContain('Current user')
    expect(selfRow.find('.rp-you').text()).toBe('You')
    expect(selfRow.text()).toContain('640K')
    expect(selfRow.text()).toContain('233')
    expect(wrapper.findAll('[data-testid="leaderboard-row"]')).toHaveLength(1)
  })

  // 本人行显示自己的昵称（仍带「你」标记），「当前用户」只是没有昵称时的兜底。
  it('names the viewer row after the signed-in username and keeps the badge', () => {
    authState.user = { username: 'zoe' }
    const wrapper = mountList([
      entry({ rank: 1, ordinal: 1, identity: { kind: 'self' }, is_self: true }),
    ])

    const selfRow = wrapper.find('[data-testid="leaderboard-row-self"]')
    expect(selfRow.text()).toContain('zoe')
    expect(selfRow.text()).not.toContain('Current user')
    expect(selfRow.find('.rp-you').text()).toBe('You')
  })

  it('falls back to the current-user wording when the username is blank', () => {
    authState.user = { username: '   ' }
    const wrapper = mountList([
      entry({ rank: 1, ordinal: 1, identity: { kind: 'self' }, is_self: true }),
    ])

    expect(wrapper.find('[data-testid="leaderboard-row-self"]').text()).toContain('Current user')
  })

  // anonymous 档他人条目缺席绝对值字段，MUST NOT 把缺席当成 0。
  it('renders relative percentages for anonymous peers and never a username', () => {
    const wrapper = mountList([
      entry({
        rank: 3,
        ordinal: 3,
        identity: { kind: 'anonymous' },
        total_tokens: undefined,
        successful_requests: undefined,
        cost: undefined,
        total_tokens_relative_percent: 42,
        successful_requests_relative_percent: 17,
        cost_relative_percent: 42,
      }),
    ])

    const row = wrapper.find('[data-testid="leaderboard-row"]')
    expect(row.text()).toContain('Row 3')
    expect(row.text()).toContain('42%')
    expect(row.text()).toContain('17%')
    expect(row.text()).not.toContain('alice')
    expect(row.find('.rp-lb-name .rp-anon').exists()).toBe(true)
  })

  it('renders a placeholder instead of zero when both value shapes are absent', () => {
    const wrapper = mountList([
      entry({ total_tokens: undefined, successful_requests: undefined, cost: undefined }),
    ])

    const row = wrapper.find('[data-testid="leaderboard-row"]')
    expect(row.find('.rp-lb-tok').text()).toBe('—')
    expect(row.find('.rp-lb-req').text()).toBe('—')
    expect(row.find('.rp-lb-cost').text()).toBe('—')
  })

  it('sizes the relative bar from the selected metric and keeps it in every row', () => {
    const wrapper = mountList([
      entry({ rank: 1, ordinal: 1, total_tokens: 1000 }),
      entry({ rank: 2, ordinal: 2, total_tokens: 250 }),
    ])

    const bars = wrapper.findAll('.rp-bar-fill')
    expect(bars[0].attributes('style')).toContain('width: 100%')
    expect(bars[1].attributes('style')).toContain('width: 25%')
    // 窄屏把这一格折到第二行，但它 MUST NOT 被隐藏：每一行都还带着条与百分比
    expect(wrapper.findAll('.rp-lb-barcell')).toHaveLength(2)
    expect(wrapper.findAll('.rp-pc')[1].text()).toBe('25%')
  })

  it('sizes the bar from the relative percent when absolutes are absent', () => {
    const wrapper = mountList(
      [
        entry({
          rank: 1,
          ordinal: 1,
          identity: { kind: 'anonymous' },
          total_tokens: undefined,
          successful_requests: undefined,
          cost: undefined,
          total_tokens_relative_percent: 100,
          successful_requests_relative_percent: 60,
          cost_relative_percent: 100,
        }),
        entry({
          rank: 2,
          ordinal: 2,
          identity: { kind: 'anonymous' },
          total_tokens: undefined,
          successful_requests: undefined,
          cost: undefined,
          total_tokens_relative_percent: 40,
          successful_requests_relative_percent: 100,
          cost_relative_percent: 40,
        }),
      ],
      { metric: 'successful_requests' },
    )

    const bars = wrapper.findAll('.rp-bar-fill')
    expect(bars[0].attributes('style')).toContain('width: 60%')
    expect(bars[1].attributes('style')).toContain('width: 100%')
  })

  it('marks the selected metric column as the sorted one', () => {
    const wrapper = mountList([entry()], { metric: 'successful_requests' })

    expect(
      wrapper.find('[data-testid="leaderboard-col-successful-requests"]').attributes('aria-sort'),
    ).toBe('descending')
    expect(
      wrapper.find('[data-testid="leaderboard-col-total-tokens"]').attributes('aria-sort'),
    ).toBe('none')
    expect(wrapper.find('[data-testid="leaderboard-col-successful-requests"]').text()).toBe(
      'Successful',
    )
  })

  // 第 6 列「金额」：表头随 Metric 标 aria-sort，与另两列同一套形态。
  it('marks the spend column as the sorted one', () => {
    const wrapper = mountList([entry()], { metric: 'cost' })

    expect(wrapper.find('[data-testid="leaderboard-col-cost"]').attributes('aria-sort')).toBe(
      'descending',
    )
    expect(
      wrapper.find('[data-testid="leaderboard-col-total-tokens"]').attributes('aria-sort'),
    ).toBe('none')
    expect(wrapper.find('[data-testid="leaderboard-col-cost"]').text()).toBe('Spend')
  })

  // 金额用 formatCurrency 渲染（locale 感知、带货币符号），不是裸数字。
  it('renders the spend column as a currency amount in named mode', () => {
    const wrapper = mountList([entry({ cost: 12.34 })], { metric: 'cost' })

    const row = wrapper.find('[data-testid="leaderboard-row"]')
    expect(row.find('.rp-lb-cost').text()).toBe('$12.34')
    expect(row.find('.rp-lb-cost').classes()).not.toContain('is-dim')
    // 没选中的那两列只是变淡，仍然渲染
    expect(row.find('.rp-lb-tok').classes()).toContain('is-dim')
  })

  // 匿名档：他人行只有相对第一名的百分比，本人行仍是真实金额。
  it('renders a relative percent for anonymous peers and the real amount for the viewer', () => {
    const wrapper = mountList(
      [
        entry({
          rank: 1,
          ordinal: 1,
          identity: { kind: 'anonymous' },
          total_tokens: undefined,
          successful_requests: undefined,
          cost: undefined,
          total_tokens_relative_percent: 100,
          successful_requests_relative_percent: 100,
          cost_relative_percent: 100,
        }),
        entry({
          rank: 2,
          ordinal: 2,
          identity: { kind: 'anonymous' },
          total_tokens: undefined,
          successful_requests: undefined,
          cost: undefined,
          total_tokens_relative_percent: 40,
          successful_requests_relative_percent: 40,
          cost_relative_percent: 31,
        }),
        entry({ rank: 3, ordinal: 3, identity: { kind: 'self' }, is_self: true, cost: 6.5 }),
      ],
      { metric: 'cost' },
    )

    const peers = wrapper.findAll('[data-testid="leaderboard-row"]')
    expect(peers[1].find('.rp-lb-cost').text()).toBe('31%')
    expect(peers[1].find('.rp-lb-cost').attributes('title')).toBe('31% of the top entry')
    expect(wrapper.find('[data-testid="leaderboard-row-self"]').find('.rp-lb-cost').text()).toBe(
      '$6.50',
    )
    // 相对条也按金额那一列算
    expect(wrapper.findAll('.rp-bar-fill')[1].attributes('style')).toContain('width: 31%')
  })

  // 解读句在 cost 下的分母是 site.cost，MUST NOT 回落到 tokens 那一格。
  it('computes the readout from the site spend total when the metric is cost', () => {
    const readout = mountList(namedBoard(), { site: site() })
      .find('[data-testid="leaderboard-rank-readout"]')
      .text()
    expect(readout).toBe('LEAD 2x · SHARE 70%')

    const costReadout = mountList(namedBoard(), { metric: 'cost', site: site() })
      .find('[data-testid="leaderboard-rank-readout"]')
      .text()
    // 第 1 名 $40 是第 2 名（$20）的 2 倍；前三名 $70 / 全站 $100
    expect(costReadout).toBe('LEAD 2x · SHARE 70%')

    const noSiteCost = mountList(namedBoard(), {
      metric: 'cost',
      site: site({ cost: undefined }),
    })
      .find('[data-testid="leaderboard-rank-readout"]')
      .text()
    expect(noSiteCost).toBe('LEAD 2x')
  })

  // 解读句压成一行两句，两档同形：第 1 名的绝对量、末名倍数与互换那句都已删。
  it('computes the one-line readout from the entries in named mode', () => {
    const readout = mountList(namedBoard(), { site: site() })
      .find('[data-testid="leaderboard-rank-readout"]')
      .text()

    // 第 1 名 4M 是第 2 名（2M）的 2 倍；前三名 7M / 全站 10M
    expect(readout).toBe('LEAD 2x · SHARE 70%')
    expect(readout).not.toContain('4M')
    expect(readout).not.toContain('#5')
    expect(readout).not.toContain('SWAP')
  })

  it('drops the site-share clause when the site total is absent', () => {
    const readout = mountList(namedBoard(), { site: site({ total_tokens: undefined }) })
      .find('[data-testid="leaderboard-rank-readout"]')
      .text()

    expect(readout).toBe('LEAD 2x')
    expect(readout).not.toContain('SHARE')
  })

  // anonymous 档：没有任何绝对量可说，倍数仍然算得出（相对百分比是同一把尺子）。
  it('keeps the readout free of absolute values in anonymous mode', () => {
    const anonymous: LeaderboardEntry[] = [
      entry({
        rank: 1,
        ordinal: 1,
        identity: { kind: 'anonymous' },
        total_tokens: undefined,
        successful_requests: undefined,
        cost: undefined,
        total_tokens_relative_percent: 100,
        successful_requests_relative_percent: 100,
        cost_relative_percent: 100,
      }),
      entry({
        rank: 2,
        ordinal: 2,
        identity: { kind: 'anonymous' },
        total_tokens: undefined,
        successful_requests: undefined,
        cost: undefined,
        total_tokens_relative_percent: 50,
        successful_requests_relative_percent: 20,
        cost_relative_percent: 50,
      }),
      entry({
        rank: 3,
        ordinal: 3,
        identity: { kind: 'anonymous' },
        total_tokens: undefined,
        successful_requests: undefined,
        cost: undefined,
        total_tokens_relative_percent: 25,
        successful_requests_relative_percent: 40,
        cost_relative_percent: 25,
      }),
    ]

    const readout = mountList(anonymous, { site: { participant_count: '5+' } })
      .find('[data-testid="leaderboard-rank-readout"]')
      .text()

    // 倍数由相对百分比现算（100 / 50 = 2 倍），绝对量与互换那句都不出现
    expect(readout).toBe('LEAD 2x')
    expect(readout).not.toContain('SHARE')
    expect(readout).not.toContain('SWAP')
    expect(readout).not.toContain('tokens')
  })

  it('renders no readout at all when there is nothing to compare', () => {
    const wrapper = mountList([entry()], { site: site() })

    expect(wrapper.find('[data-testid="leaderboard-rank-readout"]').exists()).toBe(false)
  })

  // 「共 N 位活跃 · 竞赛排名 · 本人行带…」整段注脚已删：表格自己已经把这些画出来了。
  it('renders no footnote paragraph under the table', () => {
    const wrapper = mountList(namedBoard(), { site: site() })

    expect(wrapper.find('[data-testid="leaderboard-rank-meta"]').exists()).toBe(false)
    expect(wrapper.find('.rp-meta').exists()).toBe(false)
    expect(wrapper.findAll('.rp-lb-foot p')).toHaveLength(1)
  })

  /**
   * 匿名档的真实形态：后端给**本人行**绝对量、给**他人行**相对第一名的百分比
   * （leaderboard_service.go 的 `renderNamed || item.isSelf`）。两种量纲 MUST NOT 混在
   * 一起比——那会算出天文数字的倍数，还会凭空断言两名互换。
   */
  it('never compares absolutes against relative percentages in anonymous mode', () => {
    const mixed: LeaderboardEntry[] = [
      entry({
        rank: 1,
        ordinal: 1,
        identity: { kind: 'anonymous' },
        total_tokens: undefined,
        successful_requests: undefined,
        cost: undefined,
        total_tokens_relative_percent: 100,
        successful_requests_relative_percent: 100,
        cost_relative_percent: 100,
      }),
      entry({
        rank: 2,
        ordinal: 2,
        identity: { kind: 'anonymous' },
        total_tokens: undefined,
        successful_requests: undefined,
        cost: undefined,
        total_tokens_relative_percent: 50,
        successful_requests_relative_percent: 50,
        cost_relative_percent: 50,
      }),
      // 本人行：绝对量，没有相对百分比
      entry({
        rank: 3,
        ordinal: 3,
        identity: { kind: 'self' },
        is_self: true,
        total_tokens: 250,
        successful_requests: 10,
      }),
      entry({
        rank: 4,
        ordinal: 4,
        identity: { kind: 'anonymous' },
        total_tokens: undefined,
        successful_requests: undefined,
        cost: undefined,
        total_tokens_relative_percent: 10,
        successful_requests_relative_percent: 25,
        cost_relative_percent: 10,
      }),
    ]

    const readout = mountList(mixed).find('[data-testid="leaderboard-rank-readout"]').text()

    // 倍数只在相对百分比那把尺子上算：100 / 50 = 2 倍
    expect(readout).toBe('LEAD 2x')
    // 本人行的绝对量 MUST NOT 出现在解读句里
    expect(readout).not.toContain('250')
  })

  // 本人不是第 1 名时前端算不出「相对第一名」（第一名的绝对量不公开）：留占位符，不是 0%。
  it('renders a placeholder bar for the viewer row when the top absolute is not public', () => {
    const wrapper = mountList([
      entry({
        rank: 1,
        ordinal: 1,
        identity: { kind: 'anonymous' },
        total_tokens: undefined,
        successful_requests: undefined,
        cost: undefined,
        total_tokens_relative_percent: 100,
        successful_requests_relative_percent: 100,
        cost_relative_percent: 100,
      }),
      entry({
        rank: 2,
        ordinal: 2,
        identity: { kind: 'self' },
        is_self: true,
        total_tokens: 250,
        successful_requests: 10,
      }),
    ])

    const selfRow = wrapper.find('[data-testid="leaderboard-row-self"]')
    expect(selfRow.find('.rp-pc').text()).toBe('—')
    expect(selfRow.find('.rp-pc').attributes('title')).toBe(
      'The top entry usage is not public in anonymous mode',
    )
    expect(selfRow.find('.rp-bar-fill').exists()).toBe(false)
    // 条那一格本身仍然在（窄屏折到第二行，MUST NOT 整格消失）
    expect(selfRow.find('.rp-lb-barcell').exists()).toBe(true)
    // 本人就是第 1 名时反过来算得出：自己 / 自己 = 100%
    const leading = mountList([
      entry({
        rank: 1,
        ordinal: 1,
        identity: { kind: 'self' },
        is_self: true,
        total_tokens: 250,
        successful_requests: 10,
      }),
      entry({
        rank: 2,
        ordinal: 2,
        identity: { kind: 'anonymous' },
        total_tokens: undefined,
        successful_requests: undefined,
        cost: undefined,
        total_tokens_relative_percent: 40,
        successful_requests_relative_percent: 40,
        cost_relative_percent: 40,
      }),
    ])
    expect(leading.find('[data-testid="leaderboard-row-self"]').find('.rp-pc').text()).toBe('100%')
  })

  /**
   * 头像（design D25）。后端只给 named 行下发 64px 小图，且只有内嵌头像才有小图；
   * 本人那张从自己的个人资料取，匿名行一张都不给——否则等于把匿名档拆穿。
   */
  it('renders the backend thumbnail for a named peer', () => {
    const thumb = 'data:image/jpeg;base64,AAAA'
    const row = mountList([
      entry({ identity: { kind: 'named', username: 'alice', avatar_url: thumb } }),
    ]).find('[data-testid="leaderboard-row"]')

    const avatar = row.find('[data-testid="leaderboard-avatar"]')
    expect(avatar.exists()).toBe(true)
    expect(avatar.get('[data-test="user-avatar-image"]').attributes('src')).toBe(thumb)
    expect(avatar.find('[data-test="user-avatar-initial"]').exists()).toBe(false)
  })

  // 外链头像（remote_url）没有小图，后端不下发：回退首字母，MUST NOT 去请求用户自填的地址。
  it('falls back to the display-name initial when a named peer has no thumbnail', () => {
    const row = mountList([entry({ identity: { kind: 'named', username: 'alice' } })]).find(
      '[data-testid="leaderboard-row"]',
    )

    const avatar = row.find('[data-testid="leaderboard-avatar"]')
    expect(avatar.find('[data-test="user-avatar-image"]').exists()).toBe(false)
    expect(avatar.get('[data-test="user-avatar-initial"]').text()).toBe('A')
  })

  // 匿名行：一张素色空圆。有图或有首字母都等于去匿名。
  it('renders a blank circle for anonymous peers', () => {
    const row = mountList([
      entry({ ordinal: 3, identity: { kind: 'anonymous' }, total_tokens: undefined }),
    ]).find('[data-testid="leaderboard-row"]')

    const avatar = row.find('[data-testid="leaderboard-avatar"]')
    expect(avatar.exists()).toBe(true)
    expect(avatar.find('[data-test="user-avatar-image"]').exists()).toBe(false)
    expect(avatar.get('[data-test="user-avatar-initial"]').text()).toBe('')
  })

  // 本人那张头像由前端从 auth store 取：后端 MUST NOT 为 self 下发。
  it('takes the viewer avatar from the auth store profile', () => {
    authState.user = { username: 'zoe', avatar_url: 'data:image/webp;base64,BBBB' }
    const selfRow = mountList([entry({ identity: { kind: 'self' }, is_self: true })]).find(
      '[data-testid="leaderboard-row-self"]',
    )

    const avatar = selfRow.find('[data-testid="leaderboard-avatar"]')
    expect(avatar.get('[data-test="user-avatar-image"]').attributes('src')).toBe(
      'data:image/webp;base64,BBBB',
    )
  })

  it('falls back to the viewer initial when the profile has no avatar', () => {
    authState.user = { username: 'zoe' }
    const selfRow = mountList([entry({ identity: { kind: 'self' }, is_self: true })]).find(
      '[data-testid="leaderboard-row-self"]',
    )

    const avatar = selfRow.find('[data-testid="leaderboard-avatar"]')
    expect(avatar.find('[data-test="user-avatar-image"]').exists()).toBe(false)
    expect(avatar.get('[data-test="user-avatar-initial"]').text()).toBe('Z')
  })

  // 名字被压缩时只省略名字本身，头像与「你」标记 MUST NOT 被裁掉。
  it('keeps the avatar out of the ellipsised name span', () => {
    const row = mountList([entry({ identity: { kind: 'named', username: 'alice' } })]).find(
      '[data-testid="leaderboard-row"]',
    )

    expect(row.get('.rp-lb-name .rp-lb-nametext').text()).toBe('alice')
    expect(row.get('.rp-lb-name .rp-lb-avatar').exists()).toBe(true)
  })

  // 榜单最长 50 行：级联下标按 4 行一档，MUST NOT 用未分档的渲染下标。
  it('buckets the cascade index so the last row does not wait three seconds', () => {
    const rows = Array.from({ length: 50 }, (_, index) =>
      entry({ rank: index + 1, ordinal: index + 1, total_tokens: 5000 - index }),
    )

    const rendered = mountList(rows).findAll('[data-testid="leaderboard-row"]')
    expect(rendered[0].attributes('style')).toContain('--i: 0')
    expect(rendered[4].attributes('style')).toContain('--i: 1')
    expect(rendered[49].attributes('style')).toContain('--i: 12')
  })
})
