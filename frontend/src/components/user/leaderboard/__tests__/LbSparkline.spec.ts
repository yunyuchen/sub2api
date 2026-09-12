import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import LbSparkline from '../LbSparkline.vue'

function mountSpark(props: Record<string, unknown>) {
  return mount(LbSparkline, { props: { points: [], ...props } })
}

function parsePoints(raw: string): Array<{ x: number; y: number }> {
  return raw
    .trim()
    .split(/\s+/)
    .map((pair) => {
      const [x, y] = pair.split(',').map(Number)
      return { x, y }
    })
}

describe('LbSparkline', () => {
  // 没有点就不画：一条代表 0 的平线会把「没有数据」说成「一直是 0」。
  it('renders nothing without any point', () => {
    const wrapper = mountSpark({ points: [] })

    expect(wrapper.find('[data-testid="leaderboard-sparkline"]').exists()).toBe(false)
  })

  it('draws one polyline vertex and one dot per value, marking the last one', () => {
    const wrapper = mountSpark({ points: [1, 5, 3, 9] })

    const points = parsePoints(wrapper.find('polyline').attributes('points') ?? '')
    expect(points).toHaveLength(4)
    // 值越大画得越高（y 越小）
    expect(points[3].y).toBeLessThan(points[0].y)

    // 一天一枚点（画板 chart_line 如此）：末点半径更大且是实心的
    const circles = wrapper.findAll('circle')
    expect(circles).toHaveLength(4)
    expect(Number(circles[0].attributes('cx'))).toBeCloseTo(points[0].x, 5)
    expect(circles[0].classes()).not.toContain('is-last')
    expect(circles[0].attributes('r')).toBe('1.8')

    const last = circles[3]
    expect(Number(last.attributes('cx'))).toBeCloseTo(points[3].x, 5)
    expect(Number(last.attributes('cy'))).toBeCloseTo(points[3].y, 5)
    expect(last.classes()).toContain('is-last')
    expect(last.attributes('r')).toBe('2.6')
  })

  // 名次走势：名次越小越靠上，因此纵轴要翻过来。
  it('puts smaller values on top when inverted', () => {
    const wrapper = mountSpark({ points: [18, 12], invert: true })

    const points = parsePoints(wrapper.find('polyline').attributes('points') ?? '')
    expect(points[1].y).toBeLessThan(points[0].y)
  })

  it('keeps a flat series on the mid line instead of collapsing it to an edge', () => {
    const wrapper = mountSpark({ points: [7, 7, 7], height: 50 })

    const points = parsePoints(wrapper.find('polyline').attributes('points') ?? '')
    for (const point of points) {
      expect(point.y).toBeCloseTo(25, 5)
    }
  })

  it('drops the filled area when fill is off and keeps it by default', () => {
    expect(mountSpark({ points: [1, 2, 3] }).find('.rp-chart-area').exists()).toBe(true)
    expect(
      mountSpark({ points: [1, 2, 3], fill: false }).find('.rp-chart-area').exists(),
    ).toBe(false)
  })

  // 线宽与端点由 `.rp-chart` 的样式给，颜色走行内样式，因此仍然压得过样式表。
  it('takes the stroke colour from the prop so it can follow the theme tokens', () => {
    const wrapper = mountSpark({ points: [1, 2], color: 'var(--accent-b)' })

    expect(wrapper.find('polyline').attributes('style')).toContain('stroke: var(--accent-b)')
    expect(wrapper.find('polyline').classes()).toContain('rp-chart-line')
    expect(wrapper.find('.rp-chart-area').classes()).toContain('rp-chart-area')
    const circles = wrapper.findAll('circle')
    expect(circles[circles.length - 1].classes()).toEqual(['rp-chart-dot', 'is-last'])
    // 非末点是空心的：描边跟色，填充留给 `.rp-chart-dot` 的 --bg
    expect(circles[0].attributes('style')).toContain('stroke: var(--accent-b)')
  })

  it('defaults to the page accent colour', () => {
    expect(mountSpark({ points: [1, 2] }).find('polyline').attributes('style')).toContain(
      'stroke: var(--accent)',
    )
  })

  // 没有 label 时是装饰性图形，不该出现在读屏的 tab 序列里。
  it('is decorative unless a label is given', () => {
    expect(mountSpark({ points: [1, 2] }).find('svg').attributes('aria-hidden')).toBe('true')

    const labelled = mountSpark({ points: [1, 2], label: 'Rank trend' })
    expect(labelled.find('svg').attributes('role')).toBe('img')
    expect(labelled.find('svg').attributes('aria-label')).toBe('Rank trend')
  })
})
