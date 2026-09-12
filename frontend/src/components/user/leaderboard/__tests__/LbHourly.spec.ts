import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbHourly from '../LbHourly.vue'
import type { LeaderboardHourlyBucket } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.insights.hourly.title': 'Today by hour',
  'leaderboard.insights.hourly.peak': 'Peak at {hour}:00',
  'leaderboard.insights.hourly.peakWithRequests': 'Peak at {hour}:00 · {count} requests',
  'leaderboard.insights.hourly.barRequests': '{hour}:00 · {count} requests',
  'leaderboard.insights.hourly.barRelative': '{hour}:00 · {percent}% of the peak',
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

/** 桶边界本来就是站点时区，前端不再做 UTC → 本地的映射。 */
const PERCENTS = [
  3, 2, 2, 1, 1, 2, 4, 10, 26, 48, 62, 70, 58, 64, 100, 88, 76, 69, 54, 46, 40, 31, 18, 9,
]

function namedHourly(): LeaderboardHourlyBucket[] {
  return PERCENTS.map((percent, hour) => ({
    hour,
    requests: percent * 4,
    relative_percent: percent,
  }))
}

function anonymousHourly(): LeaderboardHourlyBucket[] {
  return namedHourly().map(({ hour, relative_percent }) => ({ hour, relative_percent }))
}

function mountHourly(hourly: LeaderboardHourlyBucket[] | null) {
  return mount(LbHourly, { props: { hourly } })
}

describe('LbHourly', () => {
  it('renders nothing when the block is absent or empty', () => {
    expect(mountHourly(null).find('[data-testid="leaderboard-hourly"]').exists()).toBe(false)
    expect(mountHourly([]).find('[data-testid="leaderboard-hourly"]').exists()).toBe(false)
  })

  it('draws 24 bars and highlights exactly the peak hour', () => {
    const wrapper = mountHourly(namedHourly())

    expect(wrapper.findAll('.rp-hbars > span')).toHaveLength(24)
    const peaks = wrapper.findAll('[data-testid="leaderboard-hourly-peak"]')
    expect(peaks).toHaveLength(1)
    expect(peaks[0].classes()).toContain('is-peak')
    expect(peaks[0].attributes('style')).toContain('height: 100%')
    expect(wrapper.findAll('.rp-hbars > span')[0].attributes('style')).toContain('height: 3%')
    // 刻度上峰值那一格带强调
    expect(wrapper.findAll('.rp-hticks span')[14].classes()).toContain('is-on')
  })

  it('sorts the buckets by hour even when the source is out of order', () => {
    const wrapper = mountHourly([
      { hour: 9, requests: 40, relative_percent: 40 },
      { hour: 1, requests: 100, relative_percent: 100 },
    ])

    const bars = wrapper.findAll('.rp-hbars > span')
    expect(bars[0].attributes('title')).toBe('01:00 · 100 requests')
    expect(bars[1].attributes('title')).toBe('09:00 · 40 requests')
  })

  // 柱下只剩峰值那一句：「主要落在 HH:00 — HH:00」那半句已删。
  it('keeps only the peak line under the bars in named mode', () => {
    const wrapper = mountHourly(namedHourly())

    expect(wrapper.find('[data-testid="leaderboard-hourly-callout"]').text()).toBe(
      'Peak at 14:00 · 400 requests',
    )
    expect(wrapper.text()).not.toContain('Mostly between')
  })

  // anonymous 档没有站点级绝对量：峰值那句只剩小时，柱子的 title 换成相对峰值的百分比。
  it('drops every absolute count in anonymous mode', () => {
    const wrapper = mountHourly(anonymousHourly())

    expect(wrapper.find('[data-testid="leaderboard-hourly-callout"]').text()).toBe('Peak at 14:00')
    expect(wrapper.findAll('.rp-hbars > span')[14].attributes('title')).toBe(
      '14:00 · 100% of the peak',
    )
    expect(wrapper.text()).not.toContain('400 requests')
  })

  // 预聚合口径那句注脚与整段节奏说明都已删。
  it('drops the pre-aggregation caveat and the whole rhythm readout', () => {
    const wrapper = mountHourly(namedHourly())

    expect(wrapper.find('.rp-hint').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-rhythm-readout"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('placeholder')
  })

  it('hides the peak line entirely when nothing happened today', () => {
    const quiet = PERCENTS.map((_, hour) => ({ hour, requests: 0, relative_percent: 0 }))

    const wrapper = mountHourly(quiet)
    // 全 0 时没有峰值可言，那一行整行隐藏而不是写「峰值 00:00」
    expect(wrapper.find('[data-testid="leaderboard-hourly-callout"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="leaderboard-hourly-peak"]')).toHaveLength(0)
    expect(wrapper.findAll('.rp-hbars > span')[0].classes()).toContain('is-zero')
  })
})
