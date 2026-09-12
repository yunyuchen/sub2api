import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbHeatmap from '../LbHeatmap.vue'
import type { LeaderboardDailyBucket } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.insights.heatmap.title': 'Activity, last 30 days',
  'leaderboard.insights.heatmap.cellRequests': '{date} · {count} requests',
  'leaderboard.insights.heatmap.cellRelative': '{date} · {percent}% of the busiest day',
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

/** 相对百分比两档都下发，requests / total_tokens 只有 named 档有。 */
const PERCENTS = [0, 10, 40, 60, 90]

function namedDaily(): LeaderboardDailyBucket[] {
  return PERCENTS.map((percent, index) => ({
    date: `2026-09-0${index + 1}`,
    requests: percent * 12,
    total_tokens: percent * 1000,
    relative_percent: percent,
  }))
}

function anonymousDaily(): LeaderboardDailyBucket[] {
  return namedDaily().map(({ date, relative_percent }) => ({ date, relative_percent }))
}

function mountHeatmap(daily: LeaderboardDailyBucket[] | null) {
  return mount(LbHeatmap, { props: { daily } })
}

describe('LbHeatmap', () => {
  it('renders nothing when the block is absent or empty', () => {
    expect(mountHeatmap(null).find('[data-testid="leaderboard-heatmap"]').exists()).toBe(false)
    expect(mountHeatmap([]).find('[data-testid="leaderboard-heatmap"]').exists()).toBe(false)
  })

  // 色阶是同一个色相的四级明度（rp-lv1..rp-lv4，与标题的 .rp-h1 / .rp-h2 不撞名），0 档不加类，留给「这一天没有用量」。
  it('buckets the relative percentage into the four shades plus an empty level', () => {
    const cells = mountHeatmap(namedDaily()).findAll('[data-testid="leaderboard-heatmap-cell"]')

    expect(cells).toHaveLength(5)
    expect(cells.map((cell) => cell.classes().filter((name) => name.startsWith('rp-lv')))).toEqual([
      [],
      ['rp-lv1'],
      ['rp-lv2'],
      ['rp-lv3'],
      ['rp-lv4'],
    ])
  })

  // 相对百分比四舍五入成 0 但当天确有用量时仍给最低一档，MUST NOT 画成空白格。
  it('keeps a day with usage out of the empty level', () => {
    const cells = mountHeatmap([
      { date: '2026-09-01', requests: 3, total_tokens: 120, relative_percent: 0 },
    ]).findAll('[data-testid="leaderboard-heatmap-cell"]')

    expect(cells[0].classes()).toContain('rp-lv1')
  })

  // 预聚合口径那句注脚已删：格子只表达「这一天忙不忙」。
  it('drops the pre-aggregation caveat under the strip', () => {
    const wrapper = mountHeatmap(namedDaily())

    expect(wrapper.find('.rp-hint').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('placeholder')
  })

  it('takes the last 30 buckets and cascades them a week at a time', () => {
    const many = Array.from({ length: 45 }, (_, index) => ({
      date: `2026-09-${index}`,
      requests: index,
      total_tokens: index * 10,
      relative_percent: index % 101,
    }))

    const cells = mountHeatmap(many).findAll('[data-testid="leaderboard-heatmap-cell"]')
    expect(cells).toHaveLength(30)
    expect(cells[0].attributes('title')).toContain('2026-09-15')
    // 级联按周走（每 7 格一档），MUST NOT 逐格递延
    expect(cells[6].attributes('style')).toContain('--i: 0')
    expect(cells[7].attributes('style')).toContain('--i: 1')
  })

  it('labels a cell with counts in named mode and with a relative share in anonymous mode', () => {
    const named = mountHeatmap(namedDaily()).findAll('[data-testid="leaderboard-heatmap-cell"]')
    expect(named[4].attributes('title')).toBe('2026-09-05 · 1,080 requests')

    const anonymous = mountHeatmap(anonymousDaily()).findAll(
      '[data-testid="leaderboard-heatmap-cell"]',
    )
    expect(anonymous[4].attributes('title')).toBe('2026-09-05 · 90% of the busiest day')
    expect(mountHeatmap(anonymousDaily()).text()).not.toContain('1,080')
  })

  // 刻度只标起止两端，中间由格子自己说话。
  it('labels the first and the last day of the strip', () => {
    const ticks = mountHeatmap(namedDaily()).findAll('.rp-heat-ticks span')

    expect(ticks).toHaveLength(2)
    expect(ticks[0].classes()).toContain('is-start')
    expect(ticks[0].text()).toBe('09-01')
    expect(ticks[1].classes()).toContain('is-end')
    expect(ticks[1].text()).toBe('09-05')
  })
})
