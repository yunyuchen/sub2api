import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbFooter from '../LbFooter.vue'

const messages: Record<string, string> = {
  'leaderboard.footer.rebuild': 'rebuild every 5m',
  'leaderboard.footer.noMoney': 'no cost · no email',
  'leaderboard.footer.copyright': '© {year} {site}. All rights reserved.',
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

function mountFooter(
  snapshotUpdatedAt: string | null,
  timezone = 'Asia/Shanghai',
  siteName = 'Sub2API',
) {
  return mount(LbFooter, { props: { timezone, snapshotUpdatedAt, siteName } })
}

/** 四段之间隔着换行，断言前把空白折成一个空格。 */
function squash(text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

describe('LbFooter', () => {
  // 页脚是 colophon 一行四段 + 版权一行，`COLOPHON` 那个小字已删。
  it('renders a single colophon line with no rail mark', () => {
    const wrapper = mountFooter('2026-09-11T02:00:00Z')

    expect(wrapper.find('.rp-rail-mark').exists()).toBe(false)
    expect(wrapper.findAll('.rp-line')).toHaveLength(1)
    // 四段之间的空隙由 flex gap 给，文本节点之间没有空格
    expect(squash(wrapper.find('.rp-line').text())).toBe(
      'snapshot 10:00·rebuild every 5m·Asia/Shanghai·no cost · no email',
    )
  })

  // 版权是第二行：年份与站名都取运行时，MUST NOT 写死某一年或某个站名。
  it('prints the copyright from the runtime year and the site name', () => {
    const wrapper = mountFooter('2026-09-11T02:00:00Z')
    const copyright = wrapper.find('[data-testid="leaderboard-copyright"]')

    expect(copyright.exists()).toBe(true)
    expect(copyright.text()).toBe(
      `© ${new Date().getFullYear()} Sub2API. All rights reserved.`,
    )
    // 站名换一个，版权行跟着换
    expect(mountFooter(null, 'UTC', 'spool').find('[data-testid="leaderboard-copyright"]').text()).toBe(
      `© ${new Date().getFullYear()} spool. All rights reserved.`,
    )
    // 版权行排在 colophon 之后
    const html = wrapper.html()
    expect(html.indexOf('rp-line')).toBeLessThan(html.indexOf('leaderboard-copyright'))
  })

  // 一周起算日与 Metric 定义都已删：页脚只留数字与必要标签。
  it('drops the week-start rule, the metric definition and the long caveats', () => {
    const text = mountFooter('2026-09-11T02:00:00Z').text()

    expect(text).not.toContain('week starts')
    expect(text).not.toContain('successful_requests')
    expect(text).not.toContain('actual_cost')
    expect(text).not.toContain('COLOPHON')
    expect(text).not.toContain('placeholder')
  })

  // 时分按站点时区渲染：同一行里已经写着时区名，用浏览器本地时区会与它自相矛盾。
  it('formats the snapshot time in the site timezone, not the browser one', () => {
    expect(mountFooter('2026-09-11T02:00:00Z', 'UTC').text()).toContain('snapshot 02:00')
    expect(mountFooter('2026-09-11T02:00:00Z', 'America/New_York').text()).toContain(
      'snapshot 22:00',
    )
  })

  // 快照尚未生成时没有时间可写，换成字面量 `pending` 而不是留空。
  it('falls back to a pending literal when the snapshot has no timestamp', () => {
    expect(mountFooter(null).text()).toContain('snapshot pending')
  })
})
