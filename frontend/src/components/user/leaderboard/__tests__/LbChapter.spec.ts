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
  // spool 皮肤：章号是章名左侧一个独立的 chip，不再内联在 h2 里。
  it('renders the chapter number as a chip beside the chapter name', () => {
    const wrapper = mountChapter()

    expect(wrapper.find('[data-testid="leaderboard-chapter-01"]').exists()).toBe(true)
    expect(wrapper.find('.rp-h2row .rp-num').text()).toBe('01')
    expect(wrapper.find('.rp-h2').text()).toBe('Highlights today')
    // 章号已经移出 h2，h2 里只剩章名
    expect(wrapper.find('.rp-h2 .rp-num').exists()).toBe(false)
    // 章号 chip 排在章名之前
    const html = wrapper.html()
    expect(html.indexOf('rp-num')).toBeLessThan(html.indexOf('rp-h2"'))
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
    expect(html.indexOf('rp-h2row')).toBeLessThan(html.indexOf('body-mark'))
  })

  it('shows the subtitle next to the heading', () => {
    const plain = mountChapter({ subtitle: 'one holder per dimension' })
    expect(plain.find('.rp-h2sub').text()).toBe('one holder per dimension')

    expect(mountChapter().find('.rp-h2sub').exists()).toBe(false)
  })

  // 工具条插槽已删除：窗口 / 指标只在页头一处，刷新按钮在榜单窗口的标题栏里。
  it('has no tools slot any more', () => {
    const withTools = mountChapter({}, { tools: '<button class="tool-mark">refresh</button>' })

    expect(withTools.find('.tool-mark').exists()).toBe(false)
  })

  // 左侧边注栏已去掉：一章只有章头行与内容两部分，内容通栏。
  it('has no rail column: heading row then body', () => {
    const section = mountChapter().find('section')
    expect(section.classes()).toContain('rp-chapter')
    expect(section.classes()).not.toContain('rp-grid')
    expect(section.element.children).toHaveLength(2)
    expect(section.element.children[0].classList.contains('rp-h2row')).toBe(true)
    expect(section.element.children[1].classList.contains('rp-body')).toBe(true)
  })
})
