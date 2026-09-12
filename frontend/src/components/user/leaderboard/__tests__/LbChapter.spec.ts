import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import LbChapter from '../LbChapter.vue'

function mountChapter(props: Record<string, unknown> = {}, slots: Record<string, string> = {}) {
  return mount(LbChapter, {
    props: {
      num: '01',
      title: 'Highlights today',
      ...props,
    },
    slots: { default: '<p class="body-mark">chapter body</p>', ...slots },
  })
}

describe('LbChapter', () => {
  it('renders the chapter number inline before the chapter name', () => {
    const wrapper = mountChapter()

    expect(wrapper.find('[data-testid="leaderboard-chapter-01"]').exists()).toBe(true)
    expect(wrapper.find('.rp-h2 .rp-num').text()).toBe('01')
    expect(wrapper.find('.rp-h2').text()).toBe('01Highlights today')
  })

  // 章号是调用方给的固定编号，组件不做补零也不自己排序。
  it('prints the chapter number verbatim', () => {
    expect(mountChapter({ num: '07' }).find('.rp-num').text()).toBe('07')
    expect(mountChapter({ num: '07' }).find('[data-testid="leaderboard-chapter-07"]').exists()).toBe(
      true,
    )
  })

  it('renders the default slot below the heading row', () => {
    const wrapper = mountChapter()

    expect(wrapper.find('.rp-body .body-mark').text()).toBe('chapter body')
    const html = wrapper.html()
    expect(html.indexOf('rp-h2')).toBeLessThan(html.indexOf('body-mark'))
  })

  it('shows the subtitle next to the heading and lets the tools slot take that place', () => {
    const plain = mountChapter({ subtitle: 'one holder per dimension' })
    expect(plain.find('.rp-h2sub').text()).toBe('one holder per dimension')

    const withTools = mountChapter(
      { subtitle: 'one holder per dimension' },
      { tools: '<button class="tool-mark">refresh</button>' },
    )
    expect(withTools.find('.tool-mark').exists()).toBe(true)
    expect(withTools.find('.rp-h2sub').exists()).toBe(false)
  })

  // 左侧边注栏已去掉：一章只有章名行与内容两部分，内容通栏。
  it('has no rail column: heading row then body', () => {
    const section = mountChapter().find('section')
    expect(section.classes()).toContain('rp-chapter')
    expect(section.classes()).not.toContain('rp-grid')
    expect(section.element.children).toHaveLength(2)
    expect(section.element.children[0].classList.contains('rp-h2row')).toBe(true)
    expect(section.element.children[1].classList.contains('rp-body')).toBe(true)
  })
})
