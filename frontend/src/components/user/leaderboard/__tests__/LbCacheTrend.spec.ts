import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbCacheTrend from '../LbCacheTrend.vue'
import type { LeaderboardCacheToday, LeaderboardCacheTrendPoint } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.cacheTrend.note': 'Site-wide hit rate · last 14 days',
  'leaderboard.cacheTrend.sub': 'Cache reads are {rate}% of the input',
  'leaderboard.cacheTrend.subNamed': '{hits} tokens served from cache · {inputs} input in total',
  'leaderboard.cacheTrend.range': 'Today',
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

/** 比率两档都下发，两个绝对量只有 named 档有。 */
function points(): LeaderboardCacheTrendPoint[] {
  return [
    { date: '2026-08-29', cache_hit_rate: 0.44 },
    { date: '2026-08-30', cache_hit_rate: 0.51 },
    { date: '2026-08-31', cache_hit_rate: 0.712 },
  ]
}

function mountCacheTrend(
  value: LeaderboardCacheTrendPoint[] | null,
  cache: LeaderboardCacheToday | null = null,
) {
  return mount(LbCacheTrend, { props: { cache, points: value } })
}

describe('LbCacheTrend', () => {
  it('renders nothing when neither today nor the trend has data', () => {
    expect(mountCacheTrend(null).find('[data-testid="leaderboard-cache-trend"]').exists()).toBe(
      false,
    )
    expect(mountCacheTrend([]).find('[data-testid="leaderboard-cache-trend"]').exists()).toBe(false)
  })

  // 页面上只有这一个缓存区块：大号数字优先取今日命中率，而不是再画一张 `$ cache --today`。
  it('prefers today hit rate for the big number', () => {
    const wrapper = mountCacheTrend(points(), { cache_hit_rate: 0.658 })

    expect(wrapper.find('.rp-eyebrow').text()).toBe('Site-wide hit rate · last 14 days')
    expect(wrapper.find('[data-testid="leaderboard-cache-trend-rate"]').text()).toBe('65.8%')
    expect(wrapper.find('[data-testid="leaderboard-sparkline"]').exists()).toBe(true)
  })

  // 旧后端只给趋势时退到末点，而不是整块消失。
  it('falls back to the last trend point when today is missing', () => {
    const wrapper = mountCacheTrend(points())

    expect(wrapper.find('[data-testid="leaderboard-cache-trend-rate"]').text()).toBe('71.2%')
  })

  // 有趋势时说明行只剩一个「今日」标签：区间由右边的折线自己说。
  it('keeps only a today label under the big number when there is a trend', () => {
    const named = mountCacheTrend(points(), {
      cache_hit_rate: 0.712,
      cache_read_tokens: 8_600_000,
      input_tokens: 12_100_000,
    })
    expect(named.find('[data-testid="leaderboard-cache-trend-sub"]').text()).toBe('Today')
    expect(named.text()).not.toContain('range')

    const anonymous = mountCacheTrend(points(), { cache_hit_rate: 0.712 })
    expect(anonymous.find('[data-testid="leaderboard-cache-trend-sub"]').text()).toBe('Today')
    // 折线两端标出起止日期
    expect(named.findAll('.rp-axis span').map((tick) => tick.text())).toEqual(['08-29', '08-31'])
  })

  // 只有今日一个数时退到按档位二选一的口径句。
  it('falls back to the mode-dependent caption without a trend', () => {
    const named = mountCacheTrend([], {
      cache_hit_rate: 0.712,
      cache_read_tokens: 8_600_000,
      input_tokens: 12_100_000,
    })
    expect(named.find('[data-testid="leaderboard-cache-trend-sub"]').text()).toBe(
      '8.6M tokens served from cache · 12.1M input in total',
    )

    const anonymous = mountCacheTrend([], { cache_hit_rate: 0.712 })
    expect(anonymous.find('[data-testid="leaderboard-cache-trend-sub"]').text()).toBe(
      'Cache reads are 71.2% of the input',
    )
  })

  // 今日有数据、趋势缺行时只少一条折线，大号数字与说明行照常。
  it('keeps the card without a sparkline when the trend is empty', () => {
    const wrapper = mountCacheTrend([], { cache_hit_rate: 0.5 })

    expect(wrapper.find('[data-testid="leaderboard-cache-trend-rate"]').text()).toBe('50.0%')
    expect(wrapper.find('[data-testid="leaderboard-sparkline"]').exists()).toBe(false)
  })
})
