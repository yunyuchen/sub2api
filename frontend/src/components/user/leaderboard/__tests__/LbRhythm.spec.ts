import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbRhythm from '../LbRhythm.vue'

const messages: Record<string, string> = {
  'leaderboard.rhythm.note': 'Weekly rhythm · last 4 weeks',
  'leaderboard.insights.heatmap.legendLow': 'Less',
  'leaderboard.insights.heatmap.legendHigh': 'More',
  'leaderboard.rhythm.weekdays.mon': 'Mon',
  'leaderboard.rhythm.weekdays.tue': 'Tue',
  'leaderboard.rhythm.weekdays.wed': 'Wed',
  'leaderboard.rhythm.weekdays.thu': 'Thu',
  'leaderboard.rhythm.weekdays.fri': 'Fri',
  'leaderboard.rhythm.weekdays.sat': 'Sat',
  'leaderboard.rhythm.weekdays.sun': 'Sun',
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

/** 7 × 24 的 0–4 等级，行序周一起；两档完全相同。 */
function rhythm(): number[][] {
  return Array.from({ length: 7 }, (_, day) =>
    Array.from({ length: 24 }, (_, hour) => (hour === 14 ? 4 : (day + hour) % 5)),
  )
}

function mountRhythm(value: number[][] | null) {
  return mount(LbRhythm, { props: { rhythm: value } })
}

describe('LbRhythm', () => {
  // 整张表缺行才是「没有数据」；个别格子为 0 是「那个时段没有请求」。
  it('renders nothing when the block is absent or empty', () => {
    expect(mountRhythm(null).find('[data-testid="leaderboard-rhythm"]').exists()).toBe(false)
    expect(mountRhythm([]).find('[data-testid="leaderboard-rhythm"]').exists()).toBe(false)
  })

  it('draws seven rows of twenty-four cells with the weekday labels', () => {
    const wrapper = mountRhythm(rhythm())

    expect(wrapper.find('.rp-eyebrow').text()).toBe('Weekly rhythm · last 4 weeks')
    expect(wrapper.findAll('[data-testid="leaderboard-rhythm-cell"]')).toHaveLength(7 * 24)
    expect(wrapper.findAll('.rp-week-lbl').map((day) => day.text())).toEqual([
      'Mon',
      'Tue',
      'Wed',
      'Thu',
      'Fri',
      'Sat',
      'Sun',
    ])
  })

  it('maps the levels onto the shared colour steps and leaves level 0 bare', () => {
    const cells = mountRhythm(rhythm()).findAll('[data-testid="leaderboard-rhythm-cell"]')

    // 周一 0 点是 0 档（没有请求），周一 14 点是最高档
    expect(cells[0].attributes('class') ?? '').toBe('')
    expect(cells[1].attributes('class')).toBe('rp-lv1')
    expect(cells[14].attributes('class')).toBe('rp-lv4')
    expect(cells[0].attributes('title')).toBe('Mon 00:00')
    expect(cells[14].attributes('title')).toBe('Mon 14:00')
    // 第二行从周二开始
    expect(cells[24].attributes('title')).toBe('Tue 00:00')
  })

  it('pads short rows with the empty level instead of dropping cells', () => {
    const cells = mountRhythm([[3, 2]]).findAll('[data-testid="leaderboard-rhythm-cell"]')

    expect(cells).toHaveLength(24)
    expect(cells[0].attributes('class')).toBe('rp-lv3')
    expect(cells[2].attributes('class') ?? '').toBe('')
  })

  it('clamps out-of-range levels and keeps at most seven rows', () => {
    const cells = mountRhythm([[9, -1]]).findAll('[data-testid="leaderboard-rhythm-cell"]')
    expect(cells[0].attributes('class')).toBe('rp-lv4')
    expect(cells[1].attributes('class') ?? '').toBe('')

    const extra = [...rhythm(), Array.from({ length: 24 }, () => 4)]
    expect(mountRhythm(extra).findAll('.rp-week-lbl')).toHaveLength(7)
  })

  // 图例是「少 …… 多」的五格色阶（含 0 档），与 30 天热力共用同一套色。
  it('renders the low-to-high scale key', () => {
    const key = mountRhythm(rhythm()).find('.rp-scale-key')

    expect(key.text()).toContain('Less')
    expect(key.text()).toContain('More')
    expect(key.findAll('i')).toHaveLength(5)
  })
})
