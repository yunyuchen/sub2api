import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbHighlights from '../LbHighlights.vue'
import type { LeaderboardHighlights, LeaderboardWindow } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.highlights.topTokens.today': 'Top burner today',
  'leaderboard.highlights.topTokens.week': 'Top burner this week',
  'leaderboard.highlights.topTokens.month': 'Top burner this month',
  'leaderboard.highlights.cacheKing': 'Efficiency star',
  'leaderboard.highlights.topRequests': 'Busiest',
  'leaderboard.highlights.site.today': 'Site today',
  'leaderboard.highlights.site.week': 'Site this week',
  'leaderboard.highlights.site.month': 'Site this month',
  'leaderboard.highlights.topTokensEyebrow': '{label} · most tokens',
  'leaderboard.highlights.cacheKingEyebrow': '{label} · best cache hit rate',
  'leaderboard.highlights.topRequestsEyebrow': '{label} · most successful requests',
  'leaderboard.highlights.runnerUp': '#2',
  'leaderboard.highlights.unitTokens': 'tokens',
  'leaderboard.highlights.unitShareTokens': 'of site tokens',
  'leaderboard.highlights.unitCacheHitRate': 'cache hit rate',
  'leaderboard.highlights.unitRequests': 'successful requests',
  'leaderboard.highlights.unitShareRequests': 'of site requests',
  'leaderboard.highlights.leadSay': '{lead}% ahead of #2 · {share}% of the site',
  'leaderboard.highlights.leadSayTie': 'Tied with #2 · {share}% of the site',
  'leaderboard.highlights.leadPercent': '{percent}% ahead',
  'leaderboard.highlights.shareOfSite': '{percent}% of the site',
  'leaderboard.highlights.tiedWithSecond': 'Tied with #2',
  'leaderboard.highlights.dominantModel': 'Best: {model}',
  'leaderboard.highlights.siteRows.totalTokens': 'Total tokens',
  'leaderboard.highlights.siteRows.successfulRequests': 'Successful requests',
  'leaderboard.highlights.siteRows.participants': 'Active users',
  'leaderboard.highlights.siteRows.peakHour': 'Peak hour',
  'leaderboard.highlights.siteRows.cacheHitRate': 'Cache hit rate',
  'leaderboard.highlights.rowRequests': '{count} requests',
  'leaderboard.highlights.rowParticipants': '{count} people',
  'leaderboard.identity.anonymous': 'Row {ordinal}',
  'leaderboard.identity.outOfRank': 'Outside top 50',
  'leaderboard.table.selfBadge': 'You',
  'channelMonitorV2.currentUser': 'Current user',
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

function namedHighlights(): LeaderboardHighlights {
  return {
    top_tokens: {
      identity: { kind: 'named', username: 'alice' },
      ordinal: 1,
      total_tokens: 4_200_000,
      share_percent: 20,
      lead_percent: 45,
    },
    top_requests: {
      identity: { kind: 'named', username: 'dave' },
      ordinal: 4,
      successful_requests: 1480,
      share_percent: 23,
      lead_percent: 23,
    },
    cache_king: {
      identity: { kind: 'named', username: 'erin' },
      ordinal: 2,
      cache_hit_rate: 0.873,
      dominant_model: 'claude-sonnet-5',
    },
    site: {
      total_tokens: 21_400_000,
      successful_requests: 6412,
      participant_count: 137,
      cache_hit_rate: 0.712,
      peak_hour: 14,
      avg_tokens_per_request: 2100,
    },
  }
}

/** anonymous 档：他人与站点级绝对量一律缺席，只剩占比、相对值与两个比率。 */
function anonymousHighlights(): LeaderboardHighlights {
  return {
    top_tokens: {
      identity: { kind: 'anonymous' },
      ordinal: 1,
      share_percent: 20,
      lead_percent: 45,
    },
    top_requests: {
      identity: { kind: 'anonymous' },
      ordinal: null,
      share_percent: 23,
      lead_percent: 23,
    },
    cache_king: {
      identity: { kind: 'anonymous' },
      ordinal: 2,
      cache_hit_rate: 0.873,
      dominant_model: 'claude-sonnet-5',
    },
    site: {
      participant_count: '100+',
      cache_hit_rate: 0.712,
      peak_hour: 14,
    },
  }
}

function mountHighlights(
  highlights: LeaderboardHighlights | null,
  activeWindow: LeaderboardWindow = 'today',
) {
  return mount(LbHighlights, { props: { highlights, activeWindow } })
}

/** 大数字与单位是相邻节点，断言前把空白折成一个空格。 */
function squash(text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

describe('LbHighlights', () => {
  it('falls back to a skeleton when highlights are absent', () => {
    const wrapper = mountHighlights(null)

    expect(wrapper.find('[data-testid="leaderboard-highlights-skeleton"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-highlights"]').exists()).toBe(false)
    expect(wrapper.findAll('.rp-skel').length).toBeGreaterThan(0)
  })

  it('shows absolute values and the distance to #2 in named mode', () => {
    const wrapper = mountHighlights(namedHighlights())

    const topTokens = wrapper.find('[data-testid="leaderboard-highlight-top-tokens"]')
    expect(topTokens.text()).toContain('Top burner today · most tokens')
    expect(topTokens.text()).toContain('alice')
    expect(squash(topTokens.find('.rp-big').text())).toBe('4.2Mtokens')
    // 与第 2 名的距离由 lead_percent 反推：4.2M ÷ 1.45 ≈ 2.9M，差额 ≈ 1.3M
    const runnerUp = topTokens.find('[data-testid="leaderboard-highlight-runner-up"]')
    expect(runnerUp.text()).toContain('#2')
    expect(runnerUp.text()).toContain('2.9M')
    // 解读句两档同形，只有倍数与占比：tokens 差额那半句已删（上面两条对比条已经说了）
    expect(topTokens.find('.rp-say').text()).toBe('45% ahead of #2 · 20% of the site')

    const topRequests = wrapper.find('[data-testid="leaderboard-highlight-top-requests"]')
    expect(squash(topRequests.find('.rp-mini-v').text())).toBe('1,480successful requests')
    expect(topRequests.find('.rp-note-line').text()).toBe('23% of the site · 23% ahead')

    const site = wrapper.find('[data-testid="leaderboard-highlight-site"]')
    expect(site.text()).toContain('Site today')
    expect(site.text()).toContain('21.4M')
    expect(site.text()).toContain('6,412 requests')
    expect(site.text()).toContain('137 people')
    expect(site.text()).toContain('14:00')
    expect(site.text()).toContain('71.2%')
    // 「站点级聚合，不落到任何个人」那句小字已删
    expect(site.find('.rp-site-note').exists()).toBe(false)
    expect(site.text()).not.toContain('aggregate')
  })

  it('shows shares and pseudonyms in anonymous mode', () => {
    const wrapper = mountHighlights(anonymousHighlights())

    const topTokens = wrapper.find('[data-testid="leaderboard-highlight-top-tokens"]')
    expect(topTokens.text()).toContain('Row 1')
    expect(squash(topTokens.find('.rp-big').text())).toBe('20%of site tokens')
    expect(topTokens.find('.rp-say').text()).toBe('45% ahead of #2 · 20% of the site')
    expect(topTokens.text()).not.toContain('4.2M')
    // 第 2 名那一行也只剩相对第一名的百分比
    expect(topTokens.find('[data-testid="leaderboard-highlight-runner-up"]').text()).toContain('69%')

    const topRequests = wrapper.find('[data-testid="leaderboard-highlight-top-requests"]')
    expect(topRequests.text()).toContain('Outside top 50')
    expect(squash(topRequests.find('.rp-mini-v').text())).toBe('23%of site requests')
    expect(topRequests.find('.rp-note-line').text()).toBe('23% ahead')

    // 站点级：绝对量那两行整行缺席，比率与分档人数照常
    const site = wrapper.find('[data-testid="leaderboard-highlight-site"]')
    expect(site.text()).not.toContain('21.4M')
    expect(site.text()).not.toContain('Total tokens')
    expect(site.text()).toContain('100+')
    expect(site.text()).toContain('14:00')
    expect(site.text()).toContain('71.2%')
    expect(wrapper.findAll('.rp-ll > li')).toHaveLength(3)
  })

  // 效率之星下方那一行压成标签式：`Best: <model>`，不再是一句话。
  it('labels the dominant model instead of writing a sentence', () => {
    expect(
      mountHighlights(namedHighlights())
        .find('[data-testid="leaderboard-highlight-cache-king"]')
        .find('.rp-note-line')
        .text(),
    ).toBe('Best: claude-sonnet-5')
  })

  it('renders the cache hit rate with one decimal in both modes', () => {
    expect(
      squash(
        mountHighlights(namedHighlights())
          .find('[data-testid="leaderboard-highlight-cache-king"]')
          .find('.rp-mini-v')
          .text(),
      ),
    ).toBe('87.3%cache hit rate')
    expect(
      mountHighlights(anonymousHighlights())
        .find('[data-testid="leaderboard-highlight-cache-king"]')
        .text(),
    ).toContain('87.3%')
  })

  // 并列时不说「多 0%」，也不编一个差额出来。
  it('says the leader is tied instead of ahead by zero', () => {
    const highlights = namedHighlights()
    highlights.top_tokens = { ...highlights.top_tokens!, lead_percent: 0 }
    highlights.top_requests = { ...highlights.top_requests!, lead_percent: 0 }
    const wrapper = mountHighlights(highlights)

    expect(wrapper.find('.rp-say').text()).toBe('Tied with #2 · 20% of the site')
    expect(wrapper.find('[data-testid="leaderboard-highlight-top-requests"]').text()).toContain(
      'Tied with #2',
    )
  })

  it('hides a column whose holder is missing without touching the others', () => {
    const highlights = namedHighlights()
    highlights.cache_king = null
    const wrapper = mountHighlights(highlights)

    expect(wrapper.find('[data-testid="leaderboard-highlight-cache-king"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-highlight-top-tokens"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-highlight-site"]').exists()).toBe(true)
  })

  it('labels the window-dependent blocks from the active window', () => {
    const wrapper = mountHighlights(namedHighlights(), 'month')

    expect(wrapper.find('[data-testid="leaderboard-highlight-top-tokens"]').text()).toContain(
      'Top burner this month',
    )
    expect(wrapper.find('[data-testid="leaderboard-highlight-site"]').text()).toContain(
      'Site this month',
    )
  })

  it('names the viewer as the current user and marks the row as theirs', () => {
    const highlights = namedHighlights()
    highlights.top_tokens = { ...highlights.top_tokens!, identity: { kind: 'self' } }
    const wrapper = mountHighlights(highlights)

    const topTokens = wrapper.find('[data-testid="leaderboard-highlight-top-tokens"]')
    expect(topTokens.text()).toContain('Current user')
    expect(topTokens.find('.rp-you').text()).toBe('You')
  })

  // 本人相关的卡片也优先显示自己的昵称，「当前用户」只是没有昵称时的兜底。
  it('names the viewer after the signed-in username and keeps the badge', () => {
    authState.user = { username: 'zoe' }
    const highlights = namedHighlights()
    highlights.top_tokens = { ...highlights.top_tokens!, identity: { kind: 'self' } }
    const wrapper = mountHighlights(highlights)

    const topTokens = wrapper.find('[data-testid="leaderboard-highlight-top-tokens"]')
    expect(topTokens.text()).toContain('zoe')
    expect(topTokens.text()).not.toContain('Current user')
    expect(topTokens.find('.rp-you').text()).toBe('You')
  })

  /**
   * 头像（design D25）只挂在 tokens 领先者的 `.rp-who` 上：对比条里的 `.rp-cmp-t` 是同一个
   * 名字的第二次出现，MUST NOT 也挂一张；效率之星与最勤快那两块整块不带头像。
   */
  it('renders the top-tokens avatar once, on the who line only', () => {
    const thumb = 'data:image/jpeg;base64,AAAA'
    const highlights = namedHighlights()
    highlights.top_tokens = {
      ...highlights.top_tokens!,
      identity: { kind: 'named', username: 'alice', avatar_url: thumb },
    }
    const wrapper = mountHighlights(highlights)

    const avatars = wrapper.findAll('[data-testid="leaderboard-avatar"]')
    expect(avatars).toHaveLength(1)
    expect(avatars[0].get('[data-test="user-avatar-image"]').attributes('src')).toBe(thumb)

    const topTokens = wrapper.find('[data-testid="leaderboard-highlight-top-tokens"]')
    expect(topTokens.get('.rp-who [data-testid="leaderboard-avatar"]').exists()).toBe(true)
    expect(topTokens.find('.rp-cmp-t [data-testid="leaderboard-avatar"]').exists()).toBe(false)
    expect(
      wrapper.find('[data-testid="leaderboard-highlight-cache-king"]').find('[data-testid="leaderboard-avatar"]').exists(),
    ).toBe(false)
  })

  // 外链头像没有小图，后端不下发：回退展示名首字母。
  it('falls back to the leader initial when no thumbnail is sent', () => {
    const wrapper = mountHighlights(namedHighlights())

    const avatar = wrapper.get('[data-testid="leaderboard-avatar"]')
    expect(avatar.find('[data-test="user-avatar-image"]').exists()).toBe(false)
    expect(avatar.get('[data-test="user-avatar-initial"]').text()).toBe('A')
  })

  // 匿名档：领先者也是假名，圆圈里 MUST NOT 出现任何字符。
  it('renders a blank circle for an anonymous leader', () => {
    const wrapper = mountHighlights(anonymousHighlights())

    const avatar = wrapper.get('[data-testid="leaderboard-avatar"]')
    expect(avatar.find('[data-test="user-avatar-image"]').exists()).toBe(false)
    expect(avatar.get('[data-test="user-avatar-initial"]').text()).toBe('')
  })

  // 本人就是领先者时那张头像从 auth store 取，后端 MUST NOT 为 self 下发。
  it('takes the viewer avatar from the auth store profile', () => {
    authState.user = { username: 'zoe', avatar_url: 'data:image/webp;base64,BBBB' }
    const highlights = namedHighlights()
    highlights.top_tokens = { ...highlights.top_tokens!, identity: { kind: 'self' } }
    const wrapper = mountHighlights(highlights)

    const avatar = wrapper.get('[data-testid="leaderboard-avatar"]')
    expect(avatar.get('[data-test="user-avatar-image"]').attributes('src')).toBe(
      'data:image/webp;base64,BBBB',
    )
  })
})
