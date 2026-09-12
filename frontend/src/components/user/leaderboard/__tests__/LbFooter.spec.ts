import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbFooter from '../LbFooter.vue'

const messages: Record<string, string> = {
  'leaderboard.footer.rebuild': 'rebuild every 5m',
  'leaderboard.footer.noMoney': 'no cost · no email',
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

function mountFooter(snapshotUpdatedAt: string | null, timezone = 'Asia/Shanghai') {
  return mount(LbFooter, { props: { timezone, snapshotUpdatedAt } })
}

/** 四段之间隔着换行，断言前把空白折成一个空格。 */
function squash(text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

describe('LbFooter', () => {
  // 页脚只剩一行四段，`COLOPHON` 那个小字已删。
  it('renders a single colophon line with no rail mark', () => {
    const wrapper = mountFooter('2026-09-11T02:00:00Z')

    expect(wrapper.find('.rp-rail-mark').exists()).toBe(false)
    expect(wrapper.findAll('.rp-line')).toHaveLength(1)
    // 四段之间的空隙由 flex gap 给，文本节点之间没有空格
    expect(squash(wrapper.find('[data-testid="leaderboard-snapshot-meta"]').text())).toBe(
      'snapshot 10:00·rebuild every 5m·Asia/Shanghai·no cost · no email',
    )
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
