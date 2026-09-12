import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import LeaderboardView from '../LeaderboardView.vue'
import type {
  LeaderboardEntry,
  LeaderboardExtremes,
  LeaderboardHighlights,
  LeaderboardInsights,
  LeaderboardResponse,
  LeaderboardViewer,
} from '@/api/leaderboard'

const { getLeaderboard, showError, push } = vi.hoisted(() => ({
  getLeaderboard: vi.fn(),
  showError: vi.fn(),
  push: vi.fn(),
}))

// 只有一个只读接口，其余交互都由本地状态驱动。
vi.mock('@/api/leaderboard', () => ({
  getLeaderboard,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, siteName: 'Sub2API' }),
}))

/**
 * 展示名 helper 会读 auth store 里的 username：本人行优先显示自己的昵称，
 * 没有昵称时才回退「当前用户」。默认置空以覆盖回退分支。
 */
const authState = vi.hoisted(() => ({ user: null as { username: string } | null }))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authState }))

// 状态栏的「返回仪表盘」走站点路由，这里只关心它不炸。
vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
}))

const messages: Record<string, string> = {
  'common.refresh': 'Refresh',
  'nav.darkMode': 'Dark Mode',
  'nav.lightMode': 'Light Mode',
  'leaderboard.highlights.topTokens.today': 'Top burner today',
  'leaderboard.highlights.cacheKing': 'Efficiency star',
  'leaderboard.highlights.topRequests': 'Busiest',
  'leaderboard.highlights.site.today': 'Site today',
  'leaderboard.highlights.leadPercent': '{percent}% ahead',
  'leaderboard.highlights.dominantModel': 'Best: {model}',
  'leaderboard.extremes.nightOwl.label': 'Night owl',
  'leaderboard.extremes.rising.label': 'Rising star',
  'leaderboard.extremes.omnivore.label': 'Omnivore',
  'leaderboard.extremes.talker.label': 'Talker',
  'leaderboard.extremes.maxSingle.label': 'Largest single call',
  'leaderboard.extremes.streak.label': 'Longest streak',
  'leaderboard.whoami.note': 'Private to you',
  'leaderboard.whoami.rankTrend': 'Rank, last {span} days',
  'leaderboard.whoami.models': 'Your model mix',
  'leaderboard.whoami.compare': 'You vs the site',
  'leaderboard.whoami.cacheHitRate': 'Cache hit rate',
  'leaderboard.whoami.avgTokens': 'Tokens per request',
  'leaderboard.whoami.siteValue': 'site {value}',
  'leaderboard.windows.label': 'Window',
  'leaderboard.windows.today': 'Today',
  'leaderboard.windows.week': 'This week',
  'leaderboard.windows.month': 'This month',
  'leaderboard.metrics.label': 'Metric',
  'leaderboard.metrics.totalTokens': 'Total tokens',
  'leaderboard.metrics.successfulRequests': 'Successful requests',
  'leaderboard.masthead.brand': 'Leaderboard',
  'leaderboard.masthead.modeAnonymous': 'Anonymous',
  'leaderboard.masthead.rebuildIn': '(rebuilds in {minutes}m)',
  'leaderboard.masthead.metricTokens': 'tokens',
  'leaderboard.masthead.metricRequests': 'requests',
  'leaderboard.masthead.theme': 'Theme',
  'leaderboard.masthead.themeLight': 'Light',
  'leaderboard.masthead.themeDark': 'Dark',
  'leaderboard.masthead.backToDashboard': 'Back to dashboard',
  'leaderboard.titleBlock.heading.today': 'Who is using it today',
  'leaderboard.titleBlock.heading.week': 'Who is using it this week',
  'leaderboard.titleBlock.heading.month': 'Who is using it this month',
  'leaderboard.titleBlock.participantsUnit': 'active',
  'leaderboard.chapters.01.name.today': 'Highlights today',
  'leaderboard.chapters.01.name.week': 'Highlights this week',
  'leaderboard.chapters.01.name.month': 'Highlights this month',
  'leaderboard.chapters.02.name': 'Six records',
  'leaderboard.chapters.03.name': 'Your position',
  'leaderboard.chapters.04.name': 'Board, top 50',
  'leaderboard.chapters.05.name': 'Models and platforms',
  'leaderboard.chapters.06.name': 'Activity rhythm',
  'leaderboard.chapters.07.name': 'Trend and composition',
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
  'leaderboard.highlights.shareOfSite': '{percent}% of the site',
  'leaderboard.highlights.tiedWithSecond': 'Tied with #2',
  'leaderboard.highlights.siteRows.totalTokens': 'Total tokens',
  'leaderboard.highlights.siteRows.successfulRequests': 'Successful requests',
  'leaderboard.highlights.siteRows.participants': 'Active users',
  'leaderboard.highlights.siteRows.peakHour': 'Peak hour',
  'leaderboard.highlights.siteRows.cacheHitRate': 'Cache hit rate',
  'leaderboard.highlights.rowRequests': '{count} requests',
  'leaderboard.highlights.rowParticipants': '{count} people',
  'leaderboard.extremes.nightOwl.unit': 'of their own tokens between 0:00 and 6:00',
  'leaderboard.extremes.rising.unit': 'vs yesterday',
  'leaderboard.extremes.omnivore.unit': 'different models',
  'leaderboard.extremes.talker.unit': 'output token share',
  'leaderboard.extremes.maxSingle.unit': 'tokens per request',
  'leaderboard.extremes.maxSingle.ratioUnit': '× the median peak',
  'leaderboard.extremes.maxSingle.medianNote': '{ratio}× the median',
  'leaderboard.extremes.streak.unit': 'days in a row',
  'leaderboard.whoami.rankEyebrow': 'Rank',
  'leaderboard.whoami.rankSummary':
    'Best #{best} · worst #{worst} · #{best} on {days} of the last {span} days',
  'leaderboard.whoami.meLabel': 'You',
  'leaderboard.whoami.siteLabel': 'Site',
  'leaderboard.rank.readout.lead': 'The leader is {ratio}× of #2',
  'leaderboard.rank.readout.topThreeShare': 'Top three are {percent}% of the site',
  'leaderboard.table.rank': 'Rank',
  'leaderboard.table.user': 'User',
  'leaderboard.table.relativeToTop': 'Relative to #1',
  'leaderboard.table.totalTokens': 'Total tokens',
  'leaderboard.table.successfulRequestsShort': 'Successful',
  'leaderboard.table.relativeUnknown': 'Top entry usage is not public in anonymous mode',
  'leaderboard.table.relativePercent': '{percent}% of the top entry',
  'leaderboard.table.selfBadge': 'You',
  'leaderboard.identity.anonymous': 'Row {ordinal}',
  'leaderboard.identity.outOfRank': 'Outside top 50',
  'leaderboard.myRank.noUsage': 'No usage in this window',
  'leaderboard.myRank.noUsageHint': 'Once you generate usage you will get a rank.',
  'leaderboard.myRank.participants': '{count} participants',
  'leaderboard.myRank.suppressedHint':
    'Too few participants to list board rows; anonymity would be meaningless',
  'leaderboard.myRank.hint.gapTokens': '{gap} more tokens to reach the top {rank}',
  'leaderboard.myRank.hint.relative':
    'You are at {percent}% of the top entry; rank {rank} sits at {target}%',
  'leaderboard.insights.modelHeat.title': 'Models today',
  'leaderboard.insights.heatmap.title': 'Activity, last 30 days',
  'leaderboard.insights.heatmap.legendLow': 'Less',
  'leaderboard.insights.heatmap.legendHigh': 'More',
  'leaderboard.insights.heatmap.cellRequests': '{date} · {count} requests',
  'leaderboard.insights.heatmap.cellRelative': '{date} · {percent}% of the busiest day',
  'leaderboard.insights.trend.title': 'Usage trend · last 14 days',
  'leaderboard.insights.trend.sub': '{change} vs the day before',
  'leaderboard.insights.trend.monthTotal': 'Month to date',
  'leaderboard.insights.trend.monthChange': 'vs last month',
  'leaderboard.insights.trend.axisBreak.label': 'axis compressed at {value}',
  'leaderboard.insights.hourly.title': 'Today by hour',
  'leaderboard.insights.hourly.peak': 'Peak at {hour}:00',
  'leaderboard.insights.hourly.peakWithRequests': 'Peak at {hour}:00 · {count} requests',
  'leaderboard.insights.hourly.barRequests': '{hour}:00 · {count} requests',
  'leaderboard.insights.hourly.barRelative': '{hour}:00 · {percent}% of the peak',
  'leaderboard.profiles.more': 'others omitted',
  'leaderboard.profiles.note': 'Model mix · top 3',
  'leaderboard.platforms.note': 'Today requests by platform',
  'leaderboard.platforms.legendCount': '{count} requests',
  'leaderboard.platforms.other': 'Other',
  'leaderboard.rhythm.note': 'Weekly rhythm · last 4 weeks',
  'leaderboard.rhythm.weekdays.mon': 'Mon',
  'leaderboard.rhythm.weekdays.tue': 'Tue',
  'leaderboard.rhythm.weekdays.wed': 'Wed',
  'leaderboard.rhythm.weekdays.thu': 'Thu',
  'leaderboard.rhythm.weekdays.fri': 'Fri',
  'leaderboard.rhythm.weekdays.sat': 'Sat',
  'leaderboard.rhythm.weekdays.sun': 'Sun',
  'leaderboard.composition.note': 'Today token mix across the four kinds',
  'leaderboard.composition.input': 'Input',
  'leaderboard.composition.output': 'Output',
  'leaderboard.composition.cacheCreation': 'Cache creation',
  'leaderboard.composition.cacheRead': 'Cache read',
  'leaderboard.cacheTrend.note': 'Site-wide hit rate · last 14 days',
  'leaderboard.cacheTrend.sub': 'Cache reads are {rate}% of the input',
  'leaderboard.cacheTrend.subNamed': '{hits} tokens served from cache · {inputs} input in total',
  'leaderboard.cacheTrend.range': 'Today',
  'leaderboard.footer.rebuild': 'rebuild every 5m',
  'leaderboard.footer.noMoney': 'no cost · no email',
  'leaderboard.states.loadFailed': 'Failed to load the leaderboard',
  'leaderboard.states.empty': 'Nobody has any usage in this window yet',
  'leaderboard.states.emptyHint': 'The board appears as soon as the first usage is recorded.',
  'leaderboard.states.suppressed': 'Too few participants to show the board',
  'leaderboard.states.computing': 'The leaderboard is being computed',
  'leaderboard.states.computingHint': 'The snapshot is rebuilt every 5 minutes.',
  'leaderboard.states.stale': 'Data has not refreshed for over 15 minutes',
  'leaderboard.states.staleHint': 'You are looking at the previous snapshot.',
  'leaderboard.preview.banner': 'Preview: not visible to regular users',
  'leaderboard.preview.bannerHint': 'Leaderboard mode is currently Off.',
  'channelMonitorV2.currentUser': 'Current user',
}

function translate(key: string, params?: Record<string, unknown>): string {
  const template = messages[key] ?? key
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name] ?? ''))
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: translate }),
  }
})

/** 模板里的命令行被换行拆成了多个节点，断言前先把空白折成一个空格。 */
function squash(text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

function makeResponse(overrides: Partial<LeaderboardResponse> = {}): LeaderboardResponse {
  return {
    window: 'today',
    metric: 'total_tokens',
    mode: 'named',
    preview: false,
    timezone: 'Asia/Shanghai',
    status: 'ready',
    stale: false,
    snapshot_updated_at: '2026-09-11T02:00:00Z',
    participant_count: 12,
    entries: [],
    entries_suppressed: false,
    my_rank: null,
    highlights: null,
    insights: null,
    viewer: null,
    ...overrides,
  }
}

function namedEntry(overrides: Partial<LeaderboardEntry> = {}): LeaderboardEntry {
  return {
    rank: 1,
    ordinal: 1,
    identity: { kind: 'named', username: 'alice' },
    is_self: false,
    total_tokens: 1234,
    successful_requests: 42,
    ...overrides,
  }
}

function namedExtremes(): LeaderboardExtremes {
  return {
    night_owl: {
      identity: { kind: 'named', username: 'grace' },
      ordinal: 9,
      night_tokens: 620_000,
      night_share_percent: 62,
    },
    rising: { identity: { kind: 'named', username: 'dave' }, ordinal: 7, change_percent: 240 },
    omnivore: { identity: { kind: 'anonymous' }, ordinal: 3, distinct_models: 6 },
    talker: { identity: { kind: 'named', username: 'alice' }, ordinal: 1, output_share_percent: 71 },
    max_single: {
      identity: { kind: 'anonymous' },
      ordinal: 5,
      max_single_tokens: 1_200_000,
      ratio_to_median: 3.2,
    },
    streak: { identity: { kind: 'named', username: 'erin' }, ordinal: 2, days: 23 },
  }
}

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
    extremes: namedExtremes(),
  }
}

function namedViewer(): LeaderboardViewer {
  return {
    rank_history: [
      { date: '2026-08-30', rank: 18 },
      { date: '2026-08-31', rank: 11 },
      { date: '2026-09-01', rank: 12 },
    ],
    models: [{ model: 'claude-sonnet-5', successful_requests: 120, share_percent: 58 }],
    cache_hit_rate: 0.71,
    avg_tokens_per_request: 3400,
  }
}

/** 站点级洞察：named 档绝对量齐备，anonymous 档只剩相对量与比率。 */
function namedInsights(): LeaderboardInsights {
  return {
    models_today: [
      { model: 'claude-sonnet-5', successful_requests: 1624, share_percent: 48 },
      { model: 'claude-opus-5', successful_requests: 802, share_percent: 24 },
    ],
    daily_30: Array.from({ length: 30 }, (_, index) => ({
      date: `2026-08-${`${index + 1}`.padStart(2, '0')}`,
      requests: 100 + index,
      total_tokens: 1_000_000 * (index + 1),
      relative_percent: Math.round(((index + 1) / 30) * 100),
    })),
    hourly_today: Array.from({ length: 24 }, (_, hour) => ({
      hour,
      requests: hour * 10,
      relative_percent: Math.round((hour / 23) * 100),
    })),
    cache_today: { cache_hit_rate: 0.712, cache_read_tokens: 8_600_000, input_tokens: 12_100_000 },
    month: { total_tokens: 165_000_000, change_percent: 18 },
    profiles: [
      {
        identity: { kind: 'named', username: 'alice' },
        ordinal: 1,
        models: [
          { model: 'claude-sonnet-5', share_percent: 62 },
          { model: 'claude-opus-5', share_percent: 25 },
        ],
      },
    ],
    platforms_today: [
      { platform: 'anthropic', successful_requests: 1979, share_percent: 58 },
      { platform: 'openai', successful_requests: 921, share_percent: 27 },
    ],
    weekly_rhythm: Array.from({ length: 7 }, (_, day) =>
      Array.from({ length: 24 }, (_, hour) => (day + hour) % 5),
    ),
    composition_today: {
      input_tokens: 8_800_000,
      output_tokens: 2_600_000,
      cache_creation_tokens: 1_900_000,
      cache_read_tokens: 8_100_000,
      input_percent: 41,
      output_percent: 12,
      cache_creation_percent: 9,
      cache_read_percent: 38,
    },
    cache_trend_14: [
      { date: '2026-08-29', cache_hit_rate: 0.44 },
      { date: '2026-08-30', cache_hit_rate: 0.712 },
    ],
  }
}

function anonymousInsights(): LeaderboardInsights {
  const named = namedInsights()
  return {
    models_today: (named.models_today ?? []).map(({ model, share_percent }) => ({
      model,
      share_percent,
    })),
    daily_30: (named.daily_30 ?? []).map(({ date, relative_percent }) => ({
      date,
      relative_percent,
    })),
    hourly_today: (named.hourly_today ?? []).map(({ hour, relative_percent }) => ({
      hour,
      relative_percent,
    })),
    cache_today: { cache_hit_rate: 0.712 },
    month: { change_percent: 18 },
    // anonymous 档：他人与站点级绝对量一律缺席，只剩占比、等级与比率。
    profiles: (named.profiles ?? []).map((profile, index) => ({
      ...profile,
      identity: { kind: 'anonymous' as const },
      ordinal: index + 1,
    })),
    platforms_today: (named.platforms_today ?? []).map(({ platform, share_percent }) => ({
      platform,
      share_percent,
    })),
    weekly_rhythm: named.weekly_rhythm ?? null,
    composition_today: {
      input_percent: 41,
      output_percent: 12,
      cache_creation_percent: 9,
      cache_read_percent: 38,
    },
    cache_trend_14: named.cache_trend_14 ?? null,
  }
}

function mountView() {
  return mount(LeaderboardView)
}

describe('user LeaderboardView', () => {
  beforeEach(() => {
    getLeaderboard.mockReset()
    showError.mockReset()
    push.mockReset()
    document.documentElement.classList.remove('dark')
    authState.user = null
    getLeaderboard.mockResolvedValue(makeResponse())
  })

  // design D15 + v3 换皮：页面不再套 AppLayout，自己从 `.rp` 根节点起渲染整页，顶部是报头。
  it('renders as a full-screen standalone page with the masthead, not inside AppLayout', async () => {
    getLeaderboard.mockResolvedValue(makeResponse({ entries: [namedEntry()] }))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.element.classList.contains('rp')).toBe(true)
    expect(wrapper.find('.app-layout').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-masthead"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-title"]').exists()).toBe(true)
    const masthead = wrapper.find('[data-testid="leaderboard-masthead"]')
    expect(masthead.text()).toContain('Sub2API')
    expect(masthead.text()).toContain('Leaderboard')
    // 报头收紧后：没有日期戳、没有 tz，实名档也不挂档位 chip
    expect(masthead.find('[data-testid="leaderboard-masthead-stamp"]').exists()).toBe(false)
    expect(masthead.find('[data-testid="leaderboard-masthead-tz"]').exists()).toBe(false)
    expect(masthead.find('[data-testid="leaderboard-masthead-mode"]').exists()).toBe(false)
    expect(masthead.text()).not.toContain('Asia/Shanghai')
    // v2 的状态栏、命令行标题与 v1 的顶栏 / hero 都已经没有了
    expect(wrapper.find('[data-testid="leaderboard-status-bar"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-prompt"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-nav"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-hero"]').exists()).toBe(false)
  })

  // 标题随窗口换一句话（窗口字面量只在报头的分段上，标题不回显）；报头分段的 aria-pressed 跟着走。
  it('renders the title block from the active window', async () => {
    getLeaderboard.mockResolvedValue(makeResponse({ entries: [namedEntry()] }))

    const wrapper = mountView()
    await flushPromises()

    expect(squash(wrapper.find('[data-testid="leaderboard-title-heading"]').text())).toBe(
      'Who is using it today',
    )
    expect(
      wrapper.find('[data-testid="leaderboard-window-today"]').attributes('aria-pressed'),
    ).toBe('true')

    getLeaderboard.mockResolvedValue(
      makeResponse({
        window: 'week',
        metric: 'successful_requests',
        entries: [namedEntry()],
        highlights: namedHighlights(),
      }),
    )
    await wrapper.find('[data-testid="leaderboard-window-week"]').trigger('click')
    await wrapper.find('[data-testid="leaderboard-metric-successful-requests"]').trigger('click')
    await flushPromises()

    expect(squash(wrapper.find('[data-testid="leaderboard-title-heading"]').text())).toBe(
      'Who is using it this week',
    )
    expect(
      wrapper.find('[data-testid="leaderboard-metric-successful-requests"]').attributes(
        'aria-pressed',
      ),
    ).toBe('true')
    // 01 章的章名也跟着窗口走
    expect(wrapper.find('[data-testid="leaderboard-chapter-01"]').text()).toContain(
      'Highlights this week',
    )
  })

  // 「正在计算」MUST NOT 渲染成 0 值：人数那一段整段略过，只留日期。
  it('drops the participant segment from the title note while the snapshot is computing', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ status: 'computing', snapshot_updated_at: null, participant_count: 0 }),
    )

    const wrapper = mountView()
    await flushPromises()

    const note = squash(wrapper.find('[data-testid="leaderboard-title-note"]').text())
    expect(note).not.toContain('0 active')
    expect(note).toMatch(/^\d{4}-\d{2}-\d{2}$/)

    getLeaderboard.mockResolvedValue(
      makeResponse({ entries: [namedEntry()], participant_count: 137 }),
    )
    const ready = mountView()
    await flushPromises()
    expect(squash(ready.find('[data-testid="leaderboard-title-note"]').text())).toContain(
      '137 active',
    )
  })

  it('keeps the block order of the confirmed mockup', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        entries: [namedEntry()],
        highlights: namedHighlights(),
        viewer: namedViewer(),
        my_rank: { rank: 12, total_tokens: 900, successful_requests: 7 },
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    const html = wrapper.html()
    const order = [
      'leaderboard-masthead',
      'leaderboard-title',
      'leaderboard-highlights',
      'leaderboard-extremes',
      'leaderboard-whoami',
      'leaderboard-rank-list',
    ].map((testid) => html.indexOf(testid))

    for (let index = 1; index < order.length; index += 1) {
      expect(order[index]).toBeGreaterThan(order[index - 1])
    }
  })

  // 七章各自的编号是固定标签，不是序号。
  it('numbers the seven chapters 01 to 07 in order', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        entries: [namedEntry()],
        highlights: namedHighlights(),
        viewer: namedViewer(),
        my_rank: { rank: 12, total_tokens: 900, successful_requests: 7 },
        insights: namedInsights(),
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('.rp-chapter .rp-num').map((num) => num.text())).toEqual([
      '01',
      '02',
      '03',
      '04',
      '05',
      '06',
      '07',
    ])
  })

  // 某一章整章不渲染时，其余章号 MUST NOT 重排：页面从 03 开始是正确行为。
  it('never renumbers the remaining chapters when the leading ones drop out', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        entries: [namedEntry()],
        // 快照已就绪却没有 highlights：01 与 02 整章消失（之最与 highlights 同住一批快照）
        highlights: null,
        viewer: namedViewer(),
        my_rank: { rank: 12, total_tokens: 900, successful_requests: 7 },
        insights: namedInsights(),
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('.rp-chapter .rp-num').map((num) => num.text())).toEqual([
      '03',
      '04',
      '05',
      '06',
      '07',
    ])
    expect(wrapper.find('[data-testid="leaderboard-chapter-01"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-chapter-04"]').exists()).toBe(true)
  })

  // Window / Metric 在报头与榜单工具条两处都有，状态同源：切一处两处一起变，只发一次请求。
  it('shares one window and metric state between the masthead and the board toolbar', async () => {
    getLeaderboard.mockResolvedValue(makeResponse({ entries: [namedEntry()] }))

    const wrapper = mountView()
    await flushPromises()

    getLeaderboard.mockClear()
    await wrapper.find('[data-testid="leaderboard-rank-window-week"]').trigger('click')
    await flushPromises()

    expect(getLeaderboard).toHaveBeenCalledTimes(1)
    expect(getLeaderboard).toHaveBeenLastCalledWith(
      expect.objectContaining({ window: 'week', metric: 'total_tokens' }),
      expect.anything(),
    )
    expect(
      wrapper.find('[data-testid="leaderboard-window-week"]').attributes('aria-pressed'),
    ).toBe('true')
    expect(
      wrapper.find('[data-testid="leaderboard-rank-window-week"]').attributes('aria-pressed'),
    ).toBe('true')

    await wrapper
      .find('[data-testid="leaderboard-rank-metric-successful-requests"]')
      .trigger('click')
    await flushPromises()

    expect(getLeaderboard).toHaveBeenLastCalledWith(
      expect.objectContaining({ window: 'week', metric: 'successful_requests' }),
      expect.anything(),
    )
    expect(
      wrapper.find('[data-testid="leaderboard-metric-successful-requests"]').attributes(
        'aria-pressed',
      ),
    ).toBe('true')
  })

  // v3 的入场是纯 CSS 级联：首帧即可读，MUST NOT 依赖可见性。
  it('never gates the entrance on visibility', async () => {
    const observe = vi.fn()
    const observer = vi.fn(() => ({ observe, unobserve: vi.fn(), disconnect: vi.fn() }))
    vi.stubGlobal('IntersectionObserver', observer)
    getLeaderboard.mockResolvedValue(
      makeResponse({ entries: [namedEntry()], highlights: namedHighlights() }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(observer).not.toHaveBeenCalled()
    expect(observe).not.toHaveBeenCalled()
    const html = wrapper.html()
    expect(html).not.toContain('gk-reveal')
    expect(html).not.toContain('gk-grown')
    // 入场只有这三个 class，级联由 --i 给
    expect(wrapper.findAll('.rp-r').length).toBeGreaterThan(0)
    vi.unstubAllGlobals()
  })

  // `prefers-reduced-motion: reduce` 时数字 MUST NOT 滚动：脚本层直接跳终值。
  it('skips the number roll-up when the viewer asks for reduced motion', async () => {
    const matchMedia = vi.fn((query: string) => ({
      matches: query.includes('prefers-reduced-motion'),
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
      onchange: null,
    }))
    vi.stubGlobal('matchMedia', matchMedia)
    getLeaderboard.mockResolvedValue(
      makeResponse({ entries: [namedEntry()], highlights: namedHighlights() }),
    )

    const wrapper = mountView()
    await flushPromises()

    // 4_200_000 的紧凑写法，没有任何中间值
    expect(wrapper.find('[data-testid="leaderboard-highlight-top-tokens"]').text()).toContain('4.2M')
    vi.unstubAllGlobals()
  })

  // v3 亮色是基色：站点处于亮色时根节点不带修饰 class，切到暗色后才追加 `.dark`。
  it('keeps the root light by default and follows the theme segment', async () => {
    getLeaderboard.mockResolvedValue(makeResponse({ entries: [namedEntry()] }))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.element.classList.contains('dark')).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-theme-light"]').attributes('aria-pressed')).toBe(
      'true',
    )

    await wrapper.find('[data-testid="leaderboard-theme-dark"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(wrapper.element.classList.contains('dark')).toBe(true)

    await wrapper.find('[data-testid="leaderboard-theme-light"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(wrapper.element.classList.contains('dark')).toBe(false)
  })

  it('requests today / total_tokens on mount and shows the skeleton until it resolves', async () => {
    let resolve!: (value: LeaderboardResponse) => void
    getLeaderboard.mockReturnValue(
      new Promise<LeaderboardResponse>((r) => {
        resolve = r
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(getLeaderboard).toHaveBeenCalledWith(
      expect.objectContaining({ window: 'today', metric: 'total_tokens' }),
      expect.anything(),
    )
    expect(wrapper.find('[data-testid="leaderboard-loading"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-rank-list"]').exists()).toBe(false)

    resolve(makeResponse({ entries: [namedEntry()] }))
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-loading"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-rank-list"]').exists()).toBe(true)
  })

  it('renders the computing state as a skeleton, not an empty board', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ status: 'computing', snapshot_updated_at: null, entries: [] }),
    )

    const wrapper = mountView()
    await flushPromises()

    const computing = wrapper.find('[data-testid="leaderboard-computing"]')
    expect(computing.exists()).toBe(true)
    expect(computing.findAll('.rp-skel').length).toBeGreaterThan(0)
    expect(wrapper.find('[data-testid="leaderboard-empty"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-suppressed"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-rank-list"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('The leaderboard is being computed')
  })

  // viewer 不出自快照，因此 computing 时照常渲染。
  it('keeps the whoami block while the snapshot is still computing', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        status: 'computing',
        snapshot_updated_at: null,
        entries: [],
        highlights: null,
        viewer: namedViewer(),
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-whoami"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-whoami-rank-history"]').text()).toContain(
      'Best #11 · worst #18',
    )
    expect(wrapper.find('[data-testid="leaderboard-whoami-computing"]').exists()).toBe(true)
    // extremes 与 highlights 同源，这时候一起不可用
    expect(wrapper.find('[data-testid="leaderboard-extremes"]').exists()).toBe(false)
  })

  // highlights 为 null（computing 或旧后端）时四张卡走骨架而不是四张空卡。
  it('falls back to highlight skeletons when highlights are absent', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ status: 'computing', snapshot_updated_at: null, highlights: null }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-highlights-skeleton"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-highlights"]').exists()).toBe(false)
  })

  it('renders the highlight columns once the snapshot carries them', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ entries: [namedEntry()], highlights: namedHighlights() }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-highlights"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-highlight-top-tokens"]').text()).toContain('alice')
    expect(wrapper.find('[data-testid="leaderboard-highlight-cache-king"]').text()).toContain(
      '87.3%',
    )
    expect(wrapper.find('[data-testid="leaderboard-highlight-site"]').text()).toContain('21.4M')
    expect(wrapper.find('[data-testid="leaderboard-rank-list"]').exists()).toBe(true)
  })

  // 六张之最卡与 highlights 同源，rising 只在 today 窗口可能有值。
  it('renders the six extreme cards and drops the ones the window cannot have', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ entries: [namedEntry()], highlights: namedHighlights() }),
    )

    const wrapper = mountView()
    await flushPromises()

    const extremes = wrapper.find('[data-testid="leaderboard-extremes"]')
    expect(extremes.exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-chapter-02"]').text()).toContain('Six records')
    expect(extremes.text()).toContain('grace')
    expect(squash(extremes.find('[data-testid="leaderboard-extreme-streak"]').text())).toContain(
      '23days in a row',
    )
    expect(wrapper.find('[data-testid="leaderboard-extreme-rising"]').exists()).toBe(true)

    const weekHighlights = namedHighlights()
    weekHighlights.extremes = { ...namedExtremes(), rising: null }
    getLeaderboard.mockResolvedValue(
      makeResponse({ window: 'week', entries: [namedEntry()], highlights: weekHighlights }),
    )
    await wrapper.find('[data-testid="leaderboard-window-week"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-extreme-rising"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-extreme-streak"]').exists()).toBe(true)
  })

  it('renders the empty state when the window is ready but nobody has usage', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ status: 'ready', entries: [], entries_suppressed: false, participant_count: 0 }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-suppressed"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-computing"]').exists()).toBe(false)
  })

  // 抑制态与空态的 entries 都是空数组，只能靠 entries_suppressed 区分。
  it('separates the suppressed state from the empty state via entries_suppressed', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        mode: 'anonymous',
        status: 'ready',
        entries: [],
        entries_suppressed: true,
        participant_count: '<5',
        my_rank: { rank: 2, total_tokens: 900, successful_requests: 7 },
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-suppressed"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-empty"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-rank-list"]').exists()).toBe(false)
    // 抑制态仍要显示本人，且提示语换成抑制态的说明（只说一遍：抑制卡上只剩结论）。
    expect(wrapper.find('[data-testid="leaderboard-whoami"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('<5 participants')
    expect(wrapper.find('[data-testid="leaderboard-whoami-hint"]').text()).toBe(
      'Too few participants to list board rows; anonymity would be meaningless',
    )
    expect(wrapper.find('[data-testid="leaderboard-suppressed"]').text()).toBe(
      'Too few participants to show the board',
    )
  })

  // 快照已就绪却没有 highlights（抑制态、空窗口、旧快照）时整行隐藏：
  // 骨架只代表「还在算」，不是空态的替身。
  it('drops the highlights row entirely when a ready snapshot carries none', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        mode: 'anonymous',
        status: 'ready',
        entries: [],
        entries_suppressed: true,
        participant_count: '<5',
        highlights: null,
        my_rank: { rank: 2, total_tokens: 900, successful_requests: 7 },
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-highlights-skeleton"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-highlights"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-extremes"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-suppressed"]').exists()).toBe(true)
  })

  it('drops the highlights row for an empty ready window as well', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ status: 'ready', entries: [], participant_count: 0, highlights: null }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-highlights-skeleton"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-empty"]').exists()).toBe(true)
  })

  it('shows the stale warning while still rendering the existing snapshot', async () => {
    getLeaderboard.mockResolvedValue(makeResponse({ stale: true, entries: [namedEntry()] }))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-stale"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-rank-list"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="leaderboard-row"]')).toHaveLength(1)
  })

  it('hides the stale warning for a fresh snapshot', async () => {
    getLeaderboard.mockResolvedValue(makeResponse({ stale: false, entries: [namedEntry()] }))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-stale"]').exists()).toBe(false)
  })

  it('renders the load failure state when the request rejects', async () => {
    getLeaderboard.mockRejectedValue({ status: 500, message: 'boom' })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-rank-list"]').exists()).toBe(false)
    expect(showError).toHaveBeenCalled()
  })

  it('highlights the viewer row and keeps its absolute values', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        mode: 'anonymous',
        entries: [
          namedEntry({
            rank: 1,
            ordinal: 1,
            identity: { kind: 'anonymous' },
            is_self: false,
            total_tokens: undefined,
            successful_requests: undefined,
            total_tokens_relative_percent: 100,
            successful_requests_relative_percent: 100,
          }),
          namedEntry({
            rank: 2,
            ordinal: 2,
            identity: { kind: 'self' },
            is_self: true,
            total_tokens: 900,
            successful_requests: 7,
          }),
        ],
        my_rank: { rank: 2, total_tokens: 900, successful_requests: 7 },
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    const selfRow = wrapper.find('[data-testid="leaderboard-row-self"]')
    expect(selfRow.exists()).toBe(true)
    expect(selfRow.classes()).toContain('is-self')
    expect(selfRow.text()).toContain('Current user')
    expect(selfRow.find('.rp-you').text()).toBe('You')
    expect(selfRow.text()).toContain('900')
    expect(wrapper.findAll('[data-testid="leaderboard-row"]')).toHaveLength(1)
  })

  // 本人行显示自己的昵称（仍带「你」标记），「当前用户」只是没有昵称时的兜底。
  it('names the viewer row after the signed-in username', async () => {
    authState.user = { username: 'zoe' }
    getLeaderboard.mockResolvedValue(
      makeResponse({
        entries: [
          namedEntry({ rank: 1, ordinal: 1, identity: { kind: 'self' }, is_self: true }),
        ],
        my_rank: { rank: 1, total_tokens: 900, successful_requests: 7 },
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    const selfRow = wrapper.find('[data-testid="leaderboard-row-self"]')
    expect(selfRow.text()).toContain('zoe')
    expect(selfRow.text()).not.toContain('Current user')
    expect(selfRow.find('.rp-you').text()).toBe('You')
  })

  // anonymous 档他人条目缺席绝对值字段，MUST NOT 把缺席当成 0。
  it('shows relative percentages instead of absolute values for anonymous peers', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        mode: 'anonymous',
        participant_count: '100+',
        entries: [
          namedEntry({
            rank: 3,
            ordinal: 3,
            identity: { kind: 'anonymous' },
            total_tokens: undefined,
            successful_requests: undefined,
            total_tokens_relative_percent: 42,
            successful_requests_relative_percent: 17,
          }),
        ],
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    const row = wrapper.find('[data-testid="leaderboard-row"]')
    expect(row.text()).toContain('Row 3')
    expect(row.text()).toContain('42%')
    expect(row.text()).toContain('17%')
    expect(row.text()).not.toContain('alice')
    expect(wrapper.find('[data-testid="leaderboard-masthead-mode"]').text()).toBe('Anonymous')
    // 档位说明那一句已删：标题块副题只剩人数与日期
    expect(wrapper.find('[data-testid="leaderboard-title-note"]').text()).not.toContain(
      'Anonymous mode',
    )
  })

  // 横幅只认后端下发的 preview 字段，MUST NOT 从 mode 反推。
  it('shows the preview banner above the title block whenever preview is true', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ mode: 'off', preview: true, entries: [namedEntry()] }),
    )

    const preview = mountView()
    await flushPromises()
    expect(preview.find('[data-testid="leaderboard-preview-banner"]').exists()).toBe(true)
    const html = preview.html()
    expect(html.indexOf('leaderboard-preview-banner')).toBeLessThan(
      html.indexOf('leaderboard-title"'),
    )
    // 档位说明已从标题块删掉：预览态只由横幅表达，副题仍是人数与日期
    expect(preview.find('[data-testid="leaderboard-title-note"]').text()).not.toContain('mode')
  })

  it('hides the preview banner whenever preview is false, whatever the mode says', async () => {
    for (const mode of ['named', 'anonymous', 'off'] as const) {
      getLeaderboard.mockResolvedValue(
        makeResponse({ mode, preview: false, entries: [namedEntry()] }),
      )
      const wrapper = mountView()
      await flushPromises()
      expect(wrapper.find('[data-testid="leaderboard-preview-banner"]').exists()).toBe(false)
    }
  })

  // 「你的位置」那一句提示用后端真实的 hint 形态：kind 自带量词，目标名次固化在 kind 名里。
  it('renders the whoami hint from the backend hint shape', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        entries: [namedEntry()],
        my_rank: {
          rank: 17,
          total_tokens: 640_000,
          successful_requests: 233,
          hint: { kind: 'tokens_to_top10', value: 24_000 },
        },
      }),
    )

    const named = mountView()
    await flushPromises()
    expect(named.find('[data-testid="leaderboard-whoami-hint"]').text()).toBe(
      '24K more tokens to reach the top 10',
    )

    getLeaderboard.mockResolvedValue(
      makeResponse({
        mode: 'anonymous',
        participant_count: '100+',
        entries: [namedEntry({ identity: { kind: 'anonymous' }, total_tokens: undefined,
          successful_requests: undefined, total_tokens_relative_percent: 100,
          successful_requests_relative_percent: 100 })],
        my_rank: {
          rank: 12,
          total_tokens: 900,
          successful_requests: 7,
          hint: { kind: 'relative_percent', self: 2, tenth: 3 },
        },
      }),
    )

    const anonymous = mountView()
    await flushPromises()
    expect(anonymous.find('[data-testid="leaderboard-whoami-hint"]').text()).toBe(
      'You are at 2% of the top entry; rank 10 sits at 3%',
    )
  })

  // 已进前 10 时后端不下发 hint，页面按名次自己补这一句。
  // 「已进前 10」那一句已删：名次那个大数字已经说完了，提示行整行不渲染。
  it('renders no hint line when the backend omits the hint for a top-10 viewer', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        entries: [namedEntry()],
        my_rank: { rank: 3, total_tokens: 900, successful_requests: 7 },
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-whoami-hint"]').exists()).toBe(false)
  })

  it('shows the no-usage hint instead of a rank when my_rank is null', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ entries: [namedEntry()], my_rank: null, participant_count: 12 }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-whoami-no-usage"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('No usage in this window')
    expect(wrapper.text()).toContain('12 participants')
  })

  // viewer 是本人数据，任何档位下都真实。
  it('renders the viewer stats untouched in anonymous mode', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        mode: 'anonymous',
        participant_count: '100+',
        entries: [namedEntry({ identity: { kind: 'anonymous' }, total_tokens: undefined,
          successful_requests: undefined, total_tokens_relative_percent: 100,
          successful_requests_relative_percent: 100 })],
        my_rank: { rank: 12, total_tokens: 900, successful_requests: 7 },
        highlights: { ...namedHighlights(), site: { participant_count: '100+', cache_hit_rate: 0.52, avg_tokens_per_request: 2100 } },
        viewer: namedViewer(),
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    const compare = wrapper.find('[data-testid="leaderboard-whoami-compare"]')
    expect(compare.text()).toContain('71%')
    expect(compare.text()).toContain('site 52%')
    expect(compare.text()).toContain('3.4K')
    expect(wrapper.find('[data-testid="leaderboard-whoami-models"]').text()).toContain(
      'claude-sonnet-5',
    )
  })

  it('reloads with the selected window and metric and highlights the sorted column', async () => {
    getLeaderboard.mockResolvedValue(makeResponse({ entries: [namedEntry()] }))

    const wrapper = mountView()
    await flushPromises()

    expect(
      wrapper.find('[data-testid="leaderboard-col-total-tokens"]').attributes('aria-sort'),
    ).toBe('descending')

    getLeaderboard.mockClear()
    getLeaderboard.mockResolvedValue(
      makeResponse({ window: 'week', metric: 'successful_requests', entries: [namedEntry()] }),
    )
    await wrapper.find('[data-testid="leaderboard-window-week"]').trigger('click')
    await flushPromises()
    expect(getLeaderboard).toHaveBeenLastCalledWith(
      expect.objectContaining({ window: 'week', metric: 'total_tokens' }),
      expect.anything(),
    )

    await wrapper.find('[data-testid="leaderboard-metric-successful-requests"]').trigger('click')
    await flushPromises()
    expect(getLeaderboard).toHaveBeenLastCalledWith(
      expect.objectContaining({ window: 'week', metric: 'successful_requests' }),
      expect.anything(),
    )
    expect(
      wrapper.find('[data-testid="leaderboard-col-successful-requests"]').attributes('aria-sort'),
    ).toBe('descending')
  })

  // 页脚是一行 colophon 四段：快照时分、重建周期、时区、隐私声明。
  it('renders the footer as a one-line colophon', async () => {
    getLeaderboard.mockResolvedValue(makeResponse({ entries: [namedEntry()] }))

    const wrapper = mountView()
    await flushPromises()

    const meta = wrapper.find('[data-testid="leaderboard-snapshot-meta"]')
    expect(meta.exists()).toBe(true)
    expect(meta.text()).toContain('snapshot 10:00')
    expect(meta.text()).toContain('rebuild every 5m')
    expect(meta.text()).toContain('Asia/Shanghai')
    expect(meta.text()).toContain('no cost · no email')
    // COLOPHON 字样、一周起算日与 Metric 定义都已删
    expect(meta.text()).not.toContain('COLOPHON')
    expect(meta.text()).not.toContain('week starts Mon')
    expect(meta.text()).not.toContain('successful_requests')
    expect(meta.text()).not.toContain('placeholder')
  })

  // 洞察区块挂在榜单之后的三章里：05 模型与平台、06 活跃节奏、07 趋势与构成。
  it('renders the insight blocks in their chapters once insights arrive', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ entries: [namedEntry()], insights: namedInsights() }),
    )

    const wrapper = mountView()
    await flushPromises()

    const models = wrapper.find('[data-testid="leaderboard-chapter-05"]')
    expect(models.find('[data-testid="leaderboard-model-heat"]').exists()).toBe(true)
    expect(models.find('[data-testid="leaderboard-profiles"]').exists()).toBe(true)
    expect(models.find('[data-testid="leaderboard-platforms"]').exists()).toBe(true)

    const activity = wrapper.find('[data-testid="leaderboard-chapter-06"]')
    expect(activity.find('[data-testid="leaderboard-heatmap"]').exists()).toBe(true)
    expect(activity.find('[data-testid="leaderboard-rhythm"]').exists()).toBe(true)
    expect(activity.find('[data-testid="leaderboard-hourly"]').exists()).toBe(true)

    const trend = wrapper.find('[data-testid="leaderboard-chapter-07"]')
    expect(trend.exists()).toBe(true)
    expect(trend.find('[data-testid="leaderboard-trend"]').exists()).toBe(true)
    expect(trend.find('[data-testid="leaderboard-composition"]').exists()).toBe(true)
    // 缓存只有一块：`$ cache --today` 已并进 `$ cache --trend 14`
    expect(trend.find('[data-testid="leaderboard-cache"]').exists()).toBe(false)
    expect(trend.findAll('[data-testid="leaderboard-cache-trend"]')).toHaveLength(1)

    expect(models.find('[data-testid="leaderboard-model-heat"]').text()).toContain('1,624')
    expect(wrapper.find('[data-testid="leaderboard-hourly-callout"]').text()).toContain(
      'Peak at 23:00 · 230 requests',
    )
    expect(wrapper.find('[data-testid="leaderboard-cache-trend-sub"]').text()).toBe('Today')
    const platformLegend = models.findAll('[data-testid="leaderboard-platforms-legend"]')
    expect(platformLegend[0].text()).toContain('anthropic')
    expect(platformLegend[0].text()).toContain('1,979 requests')
    const compositionLegend = trend.findAll('[data-testid="leaderboard-composition-legend"]')
    expect(compositionLegend[0].text()).toContain('Input')
    expect(compositionLegend[0].text()).toContain('8.8M')
    // 大号数字取今日命中率，而不是趋势末点
    expect(wrapper.find('[data-testid="leaderboard-cache-trend-rate"]').text()).toBe('71.2%')
  })

  // 来源缺行时后端按区块各自给 null，页面隐藏该区块而不是用 0 填充。
  it('hides only the insight blocks whose source is missing', async () => {
    const insights = namedInsights()
    insights.models_today = null
    insights.cache_today = null
    insights.profiles = null
    insights.weekly_rhythm = null
    getLeaderboard.mockResolvedValue(
      makeResponse({ entries: [namedEntry()], insights }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-model-heat"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-profiles"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-rhythm"]').exists()).toBe(false)
    // 同一章里的其它区块照常渲染，整章也不会因为少一块就消失
    expect(
      wrapper
        .find('[data-testid="leaderboard-chapter-05"]')
        .find('[data-testid="leaderboard-platforms"]')
        .exists(),
    ).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-heatmap"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-trend"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-hourly"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-composition"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-cache-trend"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-rank-list"]').exists()).toBe(true)
  })

  it('drops the insight chapters when the whole insights payload is absent', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({ entries: [namedEntry()], insights: null }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-chapter-05"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-chapter-06"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-chapter-07"]').exists()).toBe(false)
    // 榜单那一章的章号不因此重排
    expect(wrapper.find('[data-testid="leaderboard-chapter-04"]').exists()).toBe(true)
    // 洞察缺席不影响榜单与 footer
    expect(wrapper.find('[data-testid="leaderboard-rank-list"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="leaderboard-snapshot-meta"]').exists()).toBe(true)
  })

  // anonymous 档：洞察区块里不得出现任何站点级绝对量。
  it('keeps the insight blocks free of absolute values in anonymous mode', async () => {
    getLeaderboard.mockResolvedValue(
      makeResponse({
        mode: 'anonymous',
        participant_count: '100+',
        entries: [namedEntry({ identity: { kind: 'anonymous' }, total_tokens: undefined,
          successful_requests: undefined, total_tokens_relative_percent: 100,
          successful_requests_relative_percent: 100 })],
        insights: anonymousInsights(),
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-model-heat"]').text()).toContain('48%')
    const models = wrapper.find('[data-testid="leaderboard-chapter-05"]')
    expect(models.text()).not.toContain('1,624')
    // 画像与平台分布：占比照给，他人用户名与请求次数一律缺席
    expect(models.find('[data-testid="leaderboard-profiles"]').text()).toContain('62%')
    expect(models.text()).not.toContain('alice')
    const anonymousPlatforms = models.find('[data-testid="leaderboard-platforms"]')
    expect(anonymousPlatforms.text()).toContain('anthropic')
    expect(anonymousPlatforms.text()).toContain('58%')
    expect(models.text()).not.toContain('1,979')

    const rhythm = wrapper.find('[data-testid="leaderboard-chapter-06"]')
    expect(rhythm.find('[data-testid="leaderboard-hourly-callout"]').text()).toContain(
      'Peak at 23:00',
    )
    expect(rhythm.text()).not.toContain('230 requests')

    const trio = wrapper.find('[data-testid="leaderboard-chapter-07"]')
    expect(trio.find('[data-testid="leaderboard-trend-month"]').text()).toContain('vs last month')
    expect(trio.text()).not.toContain('165M')
    expect(trio.text()).not.toContain('8.6M')
    // Token 构成的四段占比两档都有，四个绝对量只在 named 档出现
    expect(trio.find('[data-testid="leaderboard-composition"]').text()).toContain('41%')
    expect(trio.text()).not.toContain('8.8M')
    // 命中率是比率，两档都下发
    expect(trio.find('[data-testid="leaderboard-cache-trend-rate"]').text()).toBe('71.2%')
    // 周内节奏两档完全相同
    expect(wrapper.findAll('[data-testid="leaderboard-rhythm-cell"]')).toHaveLength(7 * 24)
  })

  // v3 删掉了 CRT 开关：报头上只剩主题分段与返回仪表盘。
  it('navigates back to the dashboard from the masthead and keeps no CRT toggle', async () => {
    getLeaderboard.mockResolvedValue(makeResponse({ entries: [namedEntry()] }))

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-testid="leaderboard-crt-toggle"]').exists()).toBe(false)

    await wrapper.find('[data-testid="leaderboard-back-to-dashboard"]').trigger('click')
    expect(push).toHaveBeenCalledWith('/dashboard')
  })
})
