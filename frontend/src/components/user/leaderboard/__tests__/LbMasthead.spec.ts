import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbMasthead from '../LbMasthead.vue'
import type { LeaderboardMetric, LeaderboardWindow } from '@/api/leaderboard'
import type { LeaderboardMode } from '@/utils/featureFlags'

const { push } = vi.hoisted(() => ({ push: vi.fn() }))

vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))

const messages: Record<string, string> = {
  'nav.darkMode': 'Dark Mode',
  'nav.lightMode': 'Light Mode',
  'leaderboard.windows.label': 'Window',
  'leaderboard.windows.today': 'Today',
  'leaderboard.windows.week': 'This week',
  'leaderboard.windows.month': 'This month',
  'leaderboard.metrics.label': 'Metric',
  'leaderboard.metrics.totalTokens': 'Total tokens',
  'leaderboard.metrics.successfulRequests': 'Successful requests',
  'leaderboard.metrics.cost': 'Spend',
  'leaderboard.masthead.brand': 'Leaderboard',
  'leaderboard.masthead.modeAnonymous': 'Anonymous',
  'leaderboard.masthead.rebuildIn': '(rebuilds in {minutes}m)',
  'leaderboard.masthead.metricTokens': 'tokens',
  'leaderboard.masthead.metricRequests': 'requests',
  'leaderboard.masthead.metricCost': 'spend',
  'leaderboard.masthead.theme': 'Theme',
  'leaderboard.masthead.themeLight': 'Light',
  'leaderboard.masthead.themeDark': 'Dark',
  'leaderboard.masthead.backToDashboard': 'Back to dashboard',
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

function mountMasthead(
  props: {
    siteName?: string
    activeWindow?: LeaderboardWindow
    activeMetric?: LeaderboardMetric
    mode?: LeaderboardMode | null
    snapshotUpdatedAt?: string | null
    stale?: boolean
    timezone?: string
  } = {},
) {
  return mount(LbMasthead, {
    props: {
      siteName: 'Sub2API',
      activeWindow: 'today' as const,
      activeMetric: 'total_tokens' as const,
      mode: 'named' as const,
      snapshotUpdatedAt: '2026-09-12T04:30:00Z',
      stale: false,
      timezone: 'Asia/Shanghai',
      ...props,
    },
  })
}

describe('LbMasthead', () => {
  beforeEach(() => {
    push.mockReset()
    document.documentElement.classList.remove('dark')
    vi.useFakeTimers()
    // 快照时间 12:30（Asia/Shanghai），现在是 12:32 → 距下一次重建还有 3 分钟
    vi.setSystemTime(new Date('2026-09-12T04:32:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // 刊头是一行：粗体站名 + 一个细字，没有 LEADERBOARD 小字、没有日期行、没有时分 · tz 行。
  it('renders the brand as one line and nothing else on the left', () => {
    const wrapper = mountMasthead()

    expect(wrapper.find('[data-testid="leaderboard-masthead"]').exists()).toBe(true)
    expect(wrapper.find('.rp-brand b').text()).toBe('Sub2API')
    expect(wrapper.find('.rp-brand span').text()).toBe('Leaderboard')
    expect(wrapper.find('[data-testid="leaderboard-masthead-stamp"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-masthead-tz"]').exists()).toBe(false)
  })

  // 报头上不再印 WINDOW / METRIC 两个键名，也不再印 tz：日期进标题块，时区进页脚。
  it('drops the dial labels and the timezone from the masthead', () => {
    const text = mountMasthead().text()

    expect(text).not.toContain('window')
    expect(text).not.toContain('metric')
    expect(text).not.toContain('Asia/Shanghai')
    expect(text).not.toContain('2026-09-12')
    expect(text).not.toContain('snapshot')
  })

  // 实名档是常态，不挂任何档位 chip；匿名档挂一个小 chip。
  it('marks only the anonymous mode with a chip', () => {
    expect(mountMasthead().find('[data-testid="leaderboard-masthead-mode"]').exists()).toBe(false)
    expect(
      mountMasthead({ mode: null }).find('[data-testid="leaderboard-masthead-mode"]').exists(),
    ).toBe(false)
    expect(
      mountMasthead({ mode: 'anonymous' }).find('[data-testid="leaderboard-masthead-mode"]').text(),
    ).toBe('Anonymous')
  })

  it('marks the active window and metric with aria-pressed', () => {
    const wrapper = mountMasthead({ activeWindow: 'week', activeMetric: 'successful_requests' })

    expect(wrapper.find('[data-testid="leaderboard-window-week"]').attributes('aria-pressed')).toBe(
      'true',
    )
    expect(wrapper.find('[data-testid="leaderboard-window-today"]').attributes('aria-pressed')).toBe(
      'false',
    )
    expect(
      wrapper.find('[data-testid="leaderboard-metric-successful-requests"]').attributes(
        'aria-pressed',
      ),
    ).toBe('true')
    expect(
      wrapper.find('[data-testid="leaderboard-metric-total-tokens"]').attributes('aria-pressed'),
    ).toBe('false')
  })

  it('emits the picked window and metric instead of holding the state itself', async () => {
    const wrapper = mountMasthead()

    await wrapper.find('[data-testid="leaderboard-window-month"]').trigger('click')
    await wrapper.find('[data-testid="leaderboard-metric-successful-requests"]').trigger('click')

    expect(wrapper.emitted('select-window')?.[0]).toEqual(['month'])
    expect(wrapper.emitted('select-metric')?.[0]).toEqual(['successful_requests'])
  })

  // Metric 是三项：tokens / 成功请求 / 金额，短标签进分段、完整名字进 title。
  it('offers the spend metric alongside the other two', async () => {
    const wrapper = mountMasthead({ activeMetric: 'cost' })

    const cost = wrapper.find('[data-testid="leaderboard-metric-cost"]')
    expect(cost.text()).toBe('spend')
    expect(cost.attributes('title')).toBe('Spend')
    expect(cost.attributes('aria-pressed')).toBe('true')
    expect(
      wrapper.find('[data-testid="leaderboard-metric-total-tokens"]').attributes('aria-pressed'),
    ).toBe('false')

    await wrapper.find('[data-testid="leaderboard-metric-total-tokens"]').trigger('click')
    expect(wrapper.emitted('select-metric')?.[0]).toEqual(['total_tokens'])
  })

  // 快照 chip 只剩呼吸点 + 时分 + `(+Nm)`；`(+Nm)` 按重建周期现算，MUST NOT 写死示例值。
  it('computes the minutes left until the next rebuild', () => {
    const snapshot = mountMasthead().find('[data-testid="leaderboard-masthead-snapshot"]')

    expect(snapshot.find('.rp-pulse').exists()).toBe(true)
    expect(snapshot.text()).toContain('12:30')
    expect(snapshot.text()).toContain('(rebuilds in 3m)')
    expect(snapshot.text()).not.toContain('snapshot')
  })

  it('drops the rebuild suffix when the snapshot is missing, stale or overdue', () => {
    expect(
      mountMasthead({ snapshotUpdatedAt: null })
        .find('[data-testid="leaderboard-masthead-snapshot"]')
        .text(),
    ).toContain('pending')
    expect(
      mountMasthead({ snapshotUpdatedAt: null })
        .find('[data-testid="leaderboard-masthead-snapshot"]')
        .text(),
    ).not.toContain('rebuilds in')

    expect(
      mountMasthead({ stale: true }).find('[data-testid="leaderboard-masthead-snapshot"]').text(),
    ).not.toContain('rebuilds in')

    // 已经超过一个重建周期：这句话不再成立
    vi.setSystemTime(new Date('2026-09-12T04:39:00Z'))
    expect(
      mountMasthead().find('[data-testid="leaderboard-masthead-snapshot"]').text(),
    ).not.toContain('rebuilds in')
  })

  // 主题复用站点既有的那一套（html.dark + localStorage['theme']），切换后回到其它页面不会有两套主题。
  it('drives the site theme from the theme segment and reports it back', async () => {
    const wrapper = mountMasthead()

    expect(wrapper.find('[data-testid="leaderboard-theme-light"]').attributes('aria-pressed')).toBe(
      'true',
    )

    await wrapper.find('[data-testid="leaderboard-theme-dark"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
    expect(wrapper.emitted('theme-change')?.[0]).toEqual([true])
    expect(wrapper.find('[data-testid="leaderboard-theme-dark"]').attributes('aria-pressed')).toBe(
      'true',
    )

    await wrapper.find('[data-testid="leaderboard-theme-light"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(wrapper.emitted('theme-change')?.[1]).toEqual([false])
  })

  it('does not re-emit when the already active theme is clicked again', async () => {
    const wrapper = mountMasthead()

    await wrapper.find('[data-testid="leaderboard-theme-light"]').trigger('click')

    expect(wrapper.emitted('theme-change')).toBeUndefined()
  })

  it('navigates back to the dashboard', async () => {
    const wrapper = mountMasthead()

    await wrapper.find('[data-testid="leaderboard-back-to-dashboard"]').trigger('click')

    expect(push).toHaveBeenCalledWith('/dashboard')
  })

  // 页面上不再出现任何 unicode 符号图标：主题与返回都用内联线条图标。
  it('uses inline line icons rather than unicode glyphs', () => {
    const wrapper = mountMasthead()

    expect(wrapper.find('[data-testid="leaderboard-theme-light"] svg').attributes('data-icon')).toBe(
      'sun',
    )
    expect(wrapper.find('[data-testid="leaderboard-theme-dark"] svg').attributes('data-icon')).toBe(
      'moon',
    )
    expect(
      wrapper.find('[data-testid="leaderboard-back-to-dashboard"] svg').attributes('data-icon'),
    ).toBe('arrow-left')
    expect(wrapper.text()).not.toContain('☾')
    expect(wrapper.text()).not.toContain('☼')
    expect(wrapper.text()).not.toContain('←')
  })
})
