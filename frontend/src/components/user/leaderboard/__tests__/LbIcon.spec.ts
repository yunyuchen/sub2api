import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import LbIcon from '../LbIcon.vue'
import { LEADERBOARD_ICONS, type LbIconName } from '../icons'

function mountIcon(props: Record<string, unknown>) {
  return mount(LbIcon, { props: { name: 'clock', ...props } })
}

describe('LbIcon', () => {
  // 图标只是装饰：可访问名挂在包着它的按钮 / 文字上，图形本身不进读屏与 tab 序列。
  it('renders a decorative svg with the stroke set by currentColor', () => {
    const svg = mountIcon({}).find('svg')

    expect(svg.attributes('aria-hidden')).toBe('true')
    expect(svg.attributes('focusable')).toBe('false')
    expect(svg.attributes('fill')).toBe('none')
    expect(svg.attributes('stroke')).toBe('currentColor')
    expect(svg.attributes('stroke-width')).toBe('1.5')
    expect(svg.attributes('viewBox')).toBe('0 0 16 16')
    expect(svg.attributes('data-icon')).toBe('clock')
  })

  it('takes the box size from the prop and keeps the 16-unit viewBox', () => {
    const svg = mountIcon({ size: 13 }).find('svg')

    expect(svg.attributes('width')).toBe('13')
    expect(svg.attributes('height')).toBe('13')
    expect(svg.attributes('viewBox')).toBe('0 0 16 16')
  })

  it('draws every primitive of the named icon', () => {
    // clock 是一个圆加一条指针路径
    const clock = mountIcon({ name: 'clock' })
    expect(clock.findAll('circle')).toHaveLength(1)
    expect(clock.findAll('path')).toHaveLength(1)

    // calendar 是一个圆角矩形加一条路径
    const calendar = mountIcon({ name: 'calendar' })
    expect(calendar.findAll('rect')).toHaveLength(1)
    expect(calendar.find('rect').attributes('rx')).toBe('1.4')
    expect(calendar.findAll('path')).toHaveLength(1)

    // eye-off 是三条路径
    expect(mountIcon({ name: 'eye-off' }).findAll('path')).toHaveLength(3)
  })

  // 名字给错时画一个空 svg 而不是抛错：图标 MUST NOT 因此炸掉整块内容。
  it('renders an empty svg for an unknown name', () => {
    const wrapper = mountIcon({ name: 'not-an-icon' as LbIconName })

    expect(wrapper.find('svg').exists()).toBe(true)
    expect(wrapper.findAll('path')).toHaveLength(0)
    expect(wrapper.findAll('circle')).toHaveLength(0)
  })

  // 页面上只出现这几个名字（照画板生成器的 ICONS 字典），每个都画得出来。
  it('covers every icon the page uses', () => {
    const names: LbIconName[] = [
      'arrow-left',
      'sun',
      'moon',
      'refresh',
      'eye-off',
      'clock',
      'rows',
      'layers',
      'pulse',
      'calendar',
      'bars',
      'crown',
    ]

    expect(Object.keys(LEADERBOARD_ICONS).sort()).toEqual([...names].sort())
    for (const name of names) {
      const wrapper = mountIcon({ name })
      expect(
        wrapper.findAll('path').length +
          wrapper.findAll('circle').length +
          wrapper.findAll('rect').length,
        name,
      ).toBeGreaterThan(0)
    }
  })
})
