import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbMasthead from '../LbMasthead.vue'
import type { LeaderboardMetric, LeaderboardWindow } from '@/api/leaderboard'
import type { LeaderboardMode } from '@/utils/featureFlags'

const messages: Record<string, string> = {
  'leaderboard.windows.label': 'Window',
  'leaderboard.windows.today': 'Today',
  'leaderboard.windows.week': 'This week',
  'leaderboard.windows.month': 'This month',
  'leaderboard.metrics.label': 'Metric',
  'leaderboard.metrics.totalTokens': 'Total tokens',
  'leaderboard.metrics.successfulRequests': 'Successful requests',
  'leaderboard.metrics.cost': 'Spend',
  'leaderboard.masthead.modeAnonymous': 'Anonymous',
  // 快照标签首字母大写：组件若写死小写 `snapshot` / `pending`，下面的断言会失败。
  'leaderboard.masthead.snapshot': 'Snapshot',
  'leaderboard.masthead.snapshotPending': 'Pending',
  'leaderboard.masthead.rebuildIn': '(rebuilds in {minutes}m)',
  'leaderboard.masthead.metricTokens': 'tokens',
  'leaderboard.masthead.metricRequests': 'requests',
  'leaderboard.masthead.metricCost': 'spend',
  'leaderboard.titleBlock.heading.today': "Today's overview",
  'leaderboard.titleBlock.heading.week': "This week's overview",
  'leaderboard.titleBlock.heading.month': "This month's overview",
  'leaderboard.titleBlock.participantsUnit': 'active',
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
    activeWindow?: LeaderboardWindow
    activeMetric?: LeaderboardMetric
    mode?: LeaderboardMode | null
    snapshotUpdatedAt?: string | null
    stale?: boolean
    timezone?: string
    ready?: boolean
    participantCount?: number | string
  } = {},
) {
  return mount(LbMasthead, {
    props: {
      activeWindow: 'today' as const,
      activeMetric: 'total_tokens' as const,
      mode: 'named' as const,
      snapshotUpdatedAt: '2026-09-12T04:30:00Z',
      stale: false,
      timezone: 'Asia/Shanghai',
      ready: true,
      participantCount: 137,
      ...props,
    },
  })
}

/** 副题被换行拆成了多个节点，断言前把空白折成一个空格。 */
function squash(text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

describe('LbMasthead', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    // 快照时间 12:30（Asia/Shanghai），现在是 12:32 → 距下一次重建还有 3 分钟
    vi.setSystemTime(new Date('2026-09-12T04:32:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // 页头是 H1 + 副题 + 一条控制条；站名与「返回仪表盘」已经归站点导航，这里不再有刊头。
  it('renders the page head as a heading, a sub line and one control bar', () => {
    const wrapper = mountMasthead()

    expect(wrapper.find('[data-testid="leaderboard-masthead"]').classes()).toContain('rp-phead')
    expect(wrapper.find('.rp-phead h1').text()).toBe("Today's overview")
    expect(squash(wrapper.find('.rp-phead-sub').text())).toBe('137 active · 2026-09-12')
    expect(wrapper.findAll('.rp-ctls .rp-seg')).toHaveLength(2)
    expect(wrapper.find('[data-testid="leaderboard-masthead-stamp"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-masthead-tz"]').exists()).toBe(false)
  })

  // 刊头与主题切换都已经没有了：页头 MUST NOT 再渲染它们，也不再发主题事件。
  // 站名归站点外壳；页内跳转入口整体取消（侧边栏就是导航），因此这里也不再断言它。
  it('renders neither a brand block nor a theme switch', () => {
    const wrapper = mountMasthead()

    expect(wrapper.find('.rp-brand').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-theme-light"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-theme-dark"]').exists()).toBe(false)
    expect(wrapper.emitted('theme-change')).toBeUndefined()
  })

  // 页头上不再印时区（时区在页脚），日期只在副题上出现这一次。
  it('keeps the timezone out of the page head and prints the date once', () => {
    const wrapper = mountMasthead()
    const text = wrapper.text()

    expect(text).not.toContain('Asia/Shanghai')
    expect(text.match(/2026-09-12/g)).toHaveLength(1)
  })

  // 两组分段并排，各自带一个 mono 小标签，否则分不出哪一组是窗口、哪一组是指标。
  it('labels both segments in the control bar', () => {
    const labels = mountMasthead()
      .findAll('.rp-seg b')
      .map((node) => node.text())

    expect(labels).toEqual(['Window', 'Metric'])
  })

  // H1 只剩一句话：`· today` 那段窗口回显已删，窗口本身就印在控制条的分段上。
  it('changes the heading with the window and does not echo the window literal', () => {
    expect(squash(mountMasthead().find('[data-testid="leaderboard-title-heading"]').text())).toBe(
      "Today's overview",
    )
    expect(
      squash(
        mountMasthead({ activeWindow: 'week' })
          .find('[data-testid="leaderboard-title-heading"]')
          .text(),
      ),
    ).toBe("This week's overview")
    expect(
      squash(
        mountMasthead({ activeWindow: 'month' })
          .find('[data-testid="leaderboard-title-heading"]')
          .text(),
      ),
    ).toBe("This month's overview")
  })

  // 副题只剩两段：人数与日期。档位说明与隐私声明都不在这里。
  it('keeps only the participant count and the date in the sub line', () => {
    const note = squash(mountMasthead().find('[data-testid="leaderboard-title-note"]').text())

    // 日期按**站点时区**渲染，与页脚上的 tz 一致
    expect(note).toBe('137 active · 2026-09-12')
  })

  it('drops the mode rule and the privacy sentence from the sub line', () => {
    const named = squash(mountMasthead().find('[data-testid="leaderboard-title-note"]').text())
    const anonymous = squash(
      mountMasthead({ participantCount: '100+' })
        .find('[data-testid="leaderboard-title-note"]')
        .text(),
    )

    for (const note of [named, anonymous]) {
      expect(note).not.toContain('mode')
      expect(note).not.toContain('costs')
      expect(note).not.toContain('email')
      expect(note).not.toContain('user ID')
    }
  })

  // 「正在计算」MUST NOT 渲染成 0 值：人数那一段整段略过。
  it('drops the participant segment while the snapshot is not ready', () => {
    const note = squash(
      mountMasthead({ ready: false, participantCount: 0 })
        .find('[data-testid="leaderboard-title-note"]')
        .text(),
    )

    expect(note).not.toContain('0 active')
    expect(note).not.toContain('active')
    expect(note).toBe('2026-09-12')
  })

  // anonymous 档的人数是分档字符串（如 `100+`），照原样印出来。
  it('prints a banded participant count verbatim', () => {
    const note = squash(
      mountMasthead({ participantCount: '100+' })
        .find('[data-testid="leaderboard-title-note"]')
        .text(),
    )

    expect(note).toBe('100+ active · 2026-09-12')
  })

  // 快照还没生成时日期退回当天，而不是留空或印一个假日期。
  it('falls back to today when there is no snapshot yet', () => {
    const note = squash(
      mountMasthead({ snapshotUpdatedAt: null, ready: false })
        .find('[data-testid="leaderboard-title-note"]')
        .text(),
    )

    expect(note).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })

  // 实名档是常态，不挂任何档位 chip；匿名档挂一个小 chip，排在快照 chip 之后。
  it('marks only the anonymous mode with a chip', () => {
    expect(mountMasthead().find('[data-testid="leaderboard-masthead-mode"]').exists()).toBe(false)
    expect(
      mountMasthead({ mode: null }).find('[data-testid="leaderboard-masthead-mode"]').exists(),
    ).toBe(false)

    const anonymous = mountMasthead({ mode: 'anonymous' })
    expect(anonymous.find('[data-testid="leaderboard-masthead-mode"]').text()).toBe('Anonymous')

    const html = anonymous.html()
    expect(html.indexOf('leaderboard-masthead-snapshot')).toBeLessThan(
      html.indexOf('leaderboard-masthead-mode'),
    )
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
      wrapper
        .find('[data-testid="leaderboard-metric-successful-requests"]')
        .attributes('aria-pressed'),
    ).toBe('true')
    expect(
      wrapper.find('[data-testid="leaderboard-metric-total-tokens"]').attributes('aria-pressed'),
    ).toBe('false')
  })

  // 分段上显示 i18n 标签，发出去的仍是接口参数 today / week / month：界面文字走 i18n，
  // 只有请求参数保持英文（2026-09-14 用户「顶部也做成中文」）。
  it('labels the window segments through i18n while still emitting the api value', async () => {
    const wrapper = mountMasthead()

    expect(wrapper.find('[data-testid="leaderboard-window-today"]').text()).toBe('Today')
    expect(wrapper.find('[data-testid="leaderboard-window-week"]').text()).toBe('This week')
    expect(wrapper.find('[data-testid="leaderboard-window-month"]').text()).toBe('This month')

    await wrapper.find('[data-testid="leaderboard-window-week"]').trigger('click')
    expect(wrapper.emitted('select-window')?.[0]).toEqual(['week'])
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

  // 快照 chip：呼吸点 + 快照标签 + HH:MM + 重建倒计时；倒计时按重建周期现算，MUST NOT 写死示例值。
  it('computes the minutes left until the next rebuild', () => {
    const snapshot = mountMasthead().find('[data-testid="leaderboard-masthead-snapshot"]')

    expect(snapshot.find('.rp-pulse').exists()).toBe(true)
    expect(squash(snapshot.text())).toContain('Snapshot 12:30')
    expect(snapshot.text()).toContain('(rebuilds in 3m)')
  })

  it('drops the rebuild suffix when the snapshot is missing, stale or overdue', () => {
    expect(
      mountMasthead({ snapshotUpdatedAt: null })
        .find('[data-testid="leaderboard-masthead-snapshot"]')
        .text(),
    ).toContain('Pending')
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

  // 页面上不再出现任何 unicode 符号图标。
  it('uses no unicode glyphs', () => {
    const text = mountMasthead().text()

    expect(text).not.toContain('☾')
    expect(text).not.toContain('☼')
    expect(text).not.toContain('←')
  })
})
