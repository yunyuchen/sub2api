import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbWhoami from '../LbWhoami.vue'
import type {
  LeaderboardMyRank,
  LeaderboardSiteSummary,
  LeaderboardViewer,
} from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.whoami.note': 'Private to you',
  'leaderboard.whoami.rankTrend': 'Rank, last {span} days',
  'leaderboard.whoami.rankEyebrow': 'Rank',
  'leaderboard.whoami.rankSummary': 'Best #{best} · worst #{worst}',
  'leaderboard.whoami.meLabel': 'You',
  'leaderboard.whoami.siteLabel': 'Site',
  'leaderboard.whoami.models': 'Your model mix',
  'leaderboard.whoami.compare': 'You vs the site',
  'leaderboard.whoami.cacheHitRate': 'Cache hit rate',
  'leaderboard.whoami.avgTokens': 'Tokens per request',
  'leaderboard.whoami.siteValue': 'site {value}',
  'leaderboard.myRank.noUsage': 'No usage in this window',
  'leaderboard.myRank.noUsageHint': 'Once you generate usage you will get a rank.',
  'leaderboard.myRank.participants': '{count} participants',
  'leaderboard.myRank.suppressedHint':
    'Too few participants to list board rows; anonymity would be meaningless',
  'leaderboard.myRank.hint.gapTokens': '{gap} more tokens to reach the top {rank}',
  'leaderboard.myRank.hint.gapRequests': '{gap} more successful requests to reach the top {rank}',
  'leaderboard.myRank.hint.relative':
    'You are at {percent}% of the top entry; rank {rank} sits at {target}%',
  'leaderboard.states.computing': 'The leaderboard is being computed',
  'leaderboard.states.computingHint': 'The snapshot is rebuilt every 5 minutes.',
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

function viewer(overrides: Partial<LeaderboardViewer> = {}): LeaderboardViewer {
  return {
    rank_history: [
      { date: '2026-08-30', rank: 18 },
      { date: '2026-08-31', rank: 11 },
      { date: '2026-09-01', rank: 12 },
    ],
    models: [
      { model: 'claude-sonnet-5', successful_requests: 120, share_percent: 58 },
      { model: 'gpt-5.1', successful_requests: 44, share_percent: 22 },
      { model: 'claude-opus-5', successful_requests: 24, share_percent: 12 },
    ],
    cache_hit_rate: 0.71,
    avg_tokens_per_request: 3400,
    ...overrides,
  }
}

function site(overrides: Partial<LeaderboardSiteSummary> = {}): LeaderboardSiteSummary {
  return {
    participant_count: 137,
    cache_hit_rate: 0.52,
    avg_tokens_per_request: 2100,
    ...overrides,
  }
}

function mountWhoami(
  props: {
    myRank?: LeaderboardMyRank | null
    participantCount?: number | string
    suppressed?: boolean
    computing?: boolean
    viewer?: LeaderboardViewer | null
    site?: LeaderboardSiteSummary | null
  } = {},
) {
  return mount(LbWhoami, {
    props: {
      myRank: { rank: 12, total_tokens: 900, successful_requests: 7 },
      participantCount: 137,
      viewer: viewer(),
      site: site(),
      ...props,
    },
  })
}

describe('LbWhoami', () => {
  it('renders the position, the rank trend, the model mix and the comparison', () => {
    const wrapper = mountWhoami()

    expect(wrapper.find('[data-testid="leaderboard-whoami"]').exists()).toBe(true)
    expect(wrapper.find('.rp-rank-v').text()).toBe('#12')
    expect(wrapper.text()).toContain('137 participants')
    // 私密标记：全页只留这一处，且压成一个短标签
    expect(wrapper.find('.rp-private').text()).toContain('Private to you')

    const history = wrapper.find('[data-testid="leaderboard-whoami-rank-history"]')
    expect(history.find('[data-testid="leaderboard-sparkline"]').exists()).toBe(true)
    // 折线两端是首末日期，中间一句由数据现算（窗口长度取实际点数，不写死 14）
    expect(history.find('.rp-axis').text()).toContain('2026-08-30')
    expect(history.find('.rp-axis').text()).toContain('2026-09-01')
    // 折线标题是「近 N 天名次」（N 取实际点数），下面只剩最好 / 最差两个数
    expect(history.find('.rp-eyebrow').text()).toBe('Rank, last 3 days')
    expect(history.text()).toContain('Best #11 · worst #18')
    expect(history.text()).not.toContain('Lower is better')
    expect(history.text()).not.toContain('of the last')

    const models = wrapper.find('[data-testid="leaderboard-whoami-models"]')
    expect(models.text()).toContain('claude-sonnet-5')
    expect(models.text()).toContain('58%')
    expect(models.findAll('.rp-chip')).toHaveLength(3)
    // 第一个模型是主力，chip 带强调描边
    expect(models.findAll('.rp-chip')[0].classes()).toContain('is-lead')

    const compare = wrapper.find('[data-testid="leaderboard-whoami-compare"]')
    expect(compare.text()).toContain('71%')
    expect(compare.text()).toContain('site 52%')
    expect(compare.text()).toContain('3.4K')
    expect(compare.text()).toContain('site 2.1K')
  })

  // 双条按两者中的较大值归一：较大的那条占满，另一条按比例，MUST NOT 各自归一。
  it('normalises the two bars of a row against their own peak', () => {
    const wrapper = mountWhoami()

    const row = wrapper.findAll('.rp-vs-row')[0]
    expect(row.find('.rp-vs-me i').attributes('style')).toContain('width: 100%')
    // 全站 52% ÷ 本人 71% ≈ 73.2%
    expect(row.find('.rp-vs-all i').attributes('style')).toContain('width: 73.2%')

    const below = mountWhoami({
      viewer: viewer({ cache_hit_rate: 0.26, avg_tokens_per_request: 1000 }),
    })
    const belowRow = below.findAll('.rp-vs-row')[0]
    expect(belowRow.find('.rp-vs-me i').attributes('style')).toContain('width: 50%')
    expect(belowRow.find('.rp-vs-all i').attributes('style')).toContain('width: 100%')
  })

  // viewer 是本人数据，任何档位都真实；但零用量 / 旧后端时对应那一格要消失而不是显示 0。
  it('hides the viewer cells whose data is empty', () => {
    const empty = mountWhoami({
      viewer: { rank_history: [], models: [] },
    })

    expect(empty.find('[data-testid="leaderboard-whoami-rank-history"]').exists()).toBe(false)
    expect(empty.find('[data-testid="leaderboard-whoami-models"]').exists()).toBe(false)
    expect(empty.find('[data-testid="leaderboard-whoami-compare"]').exists()).toBe(false)
    // 位置那一格与 viewer 无关，照常渲染
    expect(empty.find('.rp-rank-v').text()).toBe('#12')
  })

  it('renders without any viewer payload at all', () => {
    const wrapper = mountWhoami({ viewer: null, site: null })

    expect(wrapper.find('[data-testid="leaderboard-whoami"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-whoami-compare"]').exists()).toBe(false)
  })

  it('keeps a comparison cell whose site counterpart is missing', () => {
    const wrapper = mountWhoami({ site: { participant_count: '100+' } })

    const compare = wrapper.find('[data-testid="leaderboard-whoami-compare"]')
    expect(compare.text()).toContain('71%')
    // 全站值缺席时不拼一句半截的对照：本人那条照画，全站那条宽度为 0
    expect(compare.text()).not.toContain('site ')
    expect(compare.find('.rp-vs-all i').attributes('style')).toContain('width: 0%')
  })

  it('renders the hint from the backend hint shape', () => {
    const gap = mountWhoami({
      myRank: {
        rank: 17,
        total_tokens: 640_000,
        successful_requests: 233,
        hint: { kind: 'tokens_to_top10', value: 24_000 },
      },
    })
    expect(gap.find('[data-testid="leaderboard-whoami-hint"]').text()).toBe(
      '24K more tokens to reach the top 10',
    )

    const relative = mountWhoami({
      myRank: {
        rank: 12,
        total_tokens: 900,
        successful_requests: 7,
        hint: { kind: 'relative_percent', self: 2, tenth: 3 },
      },
    })
    expect(relative.find('[data-testid="leaderboard-whoami-hint"]').text()).toBe(
      'You are at 2% of the top entry; rank 10 sits at 3%',
    )
  })

  // 「已进前 10，你在榜单里高亮显示」那一句已删：名次那个大数字已经说完了。
  it('renders no hint line at all when the backend omits the hint', () => {
    const wrapper = mountWhoami({
      myRank: { rank: 3, total_tokens: 900, successful_requests: 7 },
    })

    expect(wrapper.find('[data-testid="leaderboard-whoami-hint"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('highlighted on the board')
  })

  it('explains the suppressed state instead of the usual hint', () => {
    const wrapper = mountWhoami({
      suppressed: true,
      participantCount: '<5',
      myRank: { rank: 2, total_tokens: 900, successful_requests: 7 },
    })

    expect(wrapper.find('[data-testid="leaderboard-whoami-hint"]').text()).toBe(
      'Too few participants to list board rows; anonymity would be meaningless',
    )
    expect(wrapper.text()).toContain('<5 participants')
  })

  it('shows the no-usage copy when the viewer has no rank', () => {
    const wrapper = mountWhoami({ myRank: null, participantCount: 12 })

    expect(wrapper.find('[data-testid="leaderboard-whoami-no-usage"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('No usage in this window')
    expect(wrapper.text()).toContain('12 participants')
  })

  // 快照还在算的时候没有名次，那不是「本窗口暂无用量」。
  it('says the snapshot is still computing rather than claiming no usage', () => {
    const wrapper = mountWhoami({ myRank: null, computing: true })

    expect(wrapper.find('[data-testid="leaderboard-whoami-computing"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-whoami-no-usage"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('The leaderboard is being computed')
    // viewer 不出自快照，这时候照常渲染
    expect(wrapper.find('[data-testid="leaderboard-whoami-rank-history"]').exists()).toBe(true)
  })
})
