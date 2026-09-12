import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbTrend from '../LbTrend.vue'
import type { LeaderboardDailyBucket, LeaderboardMonth } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.insights.trend.title': 'Usage trend · last 14 days',
  'leaderboard.insights.trend.sub': '{change} vs the day before',
  'leaderboard.insights.trend.monthTotal': 'Month to date',
  'leaderboard.insights.trend.monthChange': 'vs last month',
  'leaderboard.insights.trend.axisBreak.label': 'axis compressed at {value}',
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

/** 20 天日桶，用来验证「只取尾部 14 条」。线性递增，最大值远不到第三大值的 5 倍。 */
function namedDaily(): LeaderboardDailyBucket[] {
  return Array.from({ length: 20 }, (_, index) => ({
    date: `2026-08-${`${index + 1}`.padStart(2, '0')}`,
    requests: 100 + index,
    total_tokens: 1_000_000 * (index + 1),
    relative_percent: Math.round(((index + 1) / 20) * 100),
  }))
}

function anonymousDaily(): LeaderboardDailyBucket[] {
  return namedDaily().map(({ date, relative_percent }) => ({ date, relative_percent }))
}

/** 由一串「百万 tokens」造 14 天日桶。 */
function dailyFromMillions(values: number[]): LeaderboardDailyBucket[] {
  const max = Math.max(...values)
  return values.map((value, index) => ({
    date: `2026-09-${`${index + 1}`.padStart(2, '0')}`,
    requests: value,
    total_tokens: value * 1_000_000,
    relative_percent: Math.round((value / max) * 100),
  }))
}

function mountTrend(
  daily: LeaderboardDailyBucket[] | null,
  month: LeaderboardMonth | null = null,
) {
  return mount(LbTrend, { props: { daily, month } })
}

function heightOf(style: string | undefined): number {
  return Number(/height:\s*([\d.]+)%/.exec(style ?? '')?.[1] ?? NaN)
}

describe('LbTrend', () => {
  it('renders nothing when the block is absent or empty', () => {
    expect(mountTrend(null).find('[data-testid="leaderboard-trend"]').exists()).toBe(false)
    expect(mountTrend([]).find('[data-testid="leaderboard-trend"]').exists()).toBe(false)
  })

  it('keeps only the last 14 buckets and shortens the tick dates', () => {
    const wrapper = mountTrend(namedDaily())

    expect(wrapper.findAll('[data-testid="leaderboard-trend-col"]')).toHaveLength(14)
    const ticks = wrapper.findAll('.rp-bar-ticks span')
    expect(ticks).toHaveLength(14)
    // 每隔一格标一个日期，免得 14 个日期糊成一片
    expect(ticks[0].text()).toBe('08-07')
    expect(ticks[1].text()).toBe('')
    expect(ticks[12].text()).toBe('08-19')
  })

  it('shows absolute tokens and the month total in named mode', () => {
    const wrapper = mountTrend(namedDaily(), { total_tokens: 165_000_000, change_percent: 18 })

    const columns = wrapper.findAll('[data-testid="leaderboard-trend-col"]')
    // 线性刻度下最高的一天是 100%，而且只有它带读数
    expect(heightOf(columns[13].find('.rp-b').attributes('style'))).toBe(100)
    expect(columns[13].find('.rp-v').text()).toBe('20M')
    expect(columns[12].find('.rp-v').text()).toBe('')
    expect(columns[13].classes()).toContain('is-hot')

    const month = wrapper.find('[data-testid="leaderboard-trend-month"]')
    expect(month.text()).toContain('Month to date')
    expect(month.text()).toContain('165M')
    // 「较前一日」由最后两天现算
    expect(month.find('.rp-month-d').text()).toBe('+5% vs the day before')
  })

  // anonymous 档没有站点级绝对量：读数换成相对最高日的百分比，本月累计换成较上月的百分比。
  it('shows relative percentages and the month change in anonymous mode', () => {
    const wrapper = mountTrend(anonymousDaily(), { change_percent: 18 })

    const columns = wrapper.findAll('[data-testid="leaderboard-trend-col"]')
    expect(columns[13].find('.rp-v').text()).toBe('100%')
    expect(wrapper.text()).not.toContain('20M')

    const month = wrapper.find('[data-testid="leaderboard-trend-month"]')
    expect(month.text()).toContain('vs last month')
    expect(month.text()).toContain('+18%')
  })

  it('signs a negative month change instead of dropping the sign', () => {
    expect(
      mountTrend(anonymousDaily(), { change_percent: -12 })
        .find('[data-testid="leaderboard-trend-month"]')
        .text(),
    ).toContain('-12%')
  })

  it('hides the month box entirely when the month block is absent', () => {
    expect(mountTrend(namedDaily()).find('[data-testid="leaderboard-trend-month"]').exists()).toBe(
      false,
    )
  })

  it('drops the day-over-day line when the day before has no usage', () => {
    const daily = dailyFromMillions([0, 3])

    expect(
      mountTrend(daily, { total_tokens: 9_000_000, change_percent: 4 })
        .find('.rp-month-d')
        .exists(),
    ).toBe(false)
  })

  // 断轴：T = 第三大的值，最大值严格大于 5T 时才压缩。
  it('compresses the axis at the third largest value when the peak dwarfs it', () => {
    const wrapper = mountTrend(dailyFromMillions([1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 3, 100]))

    const breakline = wrapper.find('[data-testid="leaderboard-trend-breakline"]')
    expect(breakline.exists()).toBe(true)
    // 标注里的 T 与算出来的第三大值一致
    expect(breakline.text()).toBe('axis compressed at 2M')
    // 图上的标注保留，图下那句解释已删
    expect(wrapper.find('[data-testid="leaderboard-trend-axis-note"]').exists()).toBe(false)

    const columns = wrapper.findAll('[data-testid="leaderboard-trend-col"]')
    // 断点以下占 66%：T 那一根正好在断点上，T 的一半就是 33%
    expect(heightOf(columns[11].find('.rp-b').attributes('style'))).toBe(66)
    expect(heightOf(columns[0].find('.rp-b').attributes('style'))).toBe(33)
    // 断点以上的一段单独归一，最高的一根到顶
    expect(heightOf(columns[13].find('.rp-b').attributes('style'))).toBe(100)
    expect(heightOf(columns[12].find('.rp-b').attributes('style'))).toBeGreaterThan(78)
    // 读数只标在压缩段之上的那几天
    expect(columns[12].find('.rp-v').text()).toBe('3M')
    expect(columns[13].find('.rp-v').text()).toBe('100M')
    expect(columns[11].find('.rp-v').text()).toBe('')
    expect(columns[12].classes()).toContain('is-hot')
    expect(columns[11].classes()).not.toContain('is-hot')
  })

  // 阈值是严格大于：恰好等于 5T 时不断轴。
  it('stays linear when the peak is exactly five times the third largest', () => {
    const wrapper = mountTrend(dailyFromMillions([1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 3, 10]))

    expect(wrapper.find('[data-testid="leaderboard-trend-breakline"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-trend-axis-note"]').exists()).toBe(false)

    const columns = wrapper.findAll('[data-testid="leaderboard-trend-col"]')
    expect(heightOf(columns[13].find('.rp-b').attributes('style'))).toBe(100)
    expect(heightOf(columns[11].find('.rp-b').attributes('style'))).toBe(20)
  })

  // 少于三个点就没有「第三大」可取，一律线性。
  it('never breaks the axis with fewer than three data points', () => {
    const wrapper = mountTrend(dailyFromMillions([1, 400]))

    expect(wrapper.find('[data-testid="leaderboard-trend-breakline"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="leaderboard-trend-col"]')).toHaveLength(2)
  })

  // 匿名档同样会断轴，只是标注里的 T 写成百分比（那一列本来就是百分比）。
  it('labels the break with a percentage when absolutes are absent', () => {
    const daily = dailyFromMillions([1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 3, 100]).map(
      ({ date, relative_percent }) => ({ date, relative_percent }),
    )

    const wrapper = mountTrend(daily)
    expect(wrapper.find('[data-testid="leaderboard-trend-breakline"]').text()).toBe(
      'axis compressed at 2%',
    )
    expect(wrapper.text()).not.toContain('2M')
  })
})
