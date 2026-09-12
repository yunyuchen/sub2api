import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbTitle from '../LbTitle.vue'
import type { LeaderboardWindow } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.titleBlock.heading.today': 'Who is using it today',
  'leaderboard.titleBlock.heading.week': 'Who is using it this week',
  'leaderboard.titleBlock.heading.month': 'Who is using it this month',
  'leaderboard.titleBlock.participantsUnit': 'active',
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

function mountTitle(
  props: {
    activeWindow?: LeaderboardWindow
    ready?: boolean
    participantCount?: number | string
    snapshotUpdatedAt?: string | null
    timezone?: string
  } = {},
) {
  return mount(LbTitle, {
    props: {
      activeWindow: 'today' as const,
      ready: true,
      participantCount: 137,
      snapshotUpdatedAt: '2026-09-12T04:30:00Z',
      timezone: 'Asia/Shanghai',
      ...props,
    },
  })
}

/** 副题被换行拆成了多个节点，断言前把空白折成一个空格。 */
function squash(text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

describe('LbTitle', () => {
  // H1 只剩一句话：`· today` 那段窗口回显已删，窗口本身就印在报头的分段上。
  it('changes the heading with the window and does not echo the window literal', () => {
    expect(
      squash(mountTitle().find('[data-testid="leaderboard-title-heading"]').text()),
    ).toBe('Who is using it today')
    expect(
      squash(
        mountTitle({ activeWindow: 'week' })
          .find('[data-testid="leaderboard-title-heading"]')
          .text(),
      ),
    ).toBe('Who is using it this week')
    expect(
      squash(
        mountTitle({ activeWindow: 'month' })
          .find('[data-testid="leaderboard-title-heading"]')
          .text(),
      ),
    ).toBe('Who is using it this month')
  })

  // 副题只剩两段：人数与日期。档位说明与隐私声明都不在这里。
  it('keeps only the participant count and the date in the sub line', () => {
    const note = squash(mountTitle().find('[data-testid="leaderboard-title-note"]').text())

    // 日期按**站点时区**渲染，与页脚上的 tz 一致（`·` 与日期之间的空隙由样式给）
    expect(note).toBe('137 active ·2026-09-12')
  })

  it('drops the mode rule and the privacy sentence from the sub line', () => {
    const named = squash(mountTitle().find('[data-testid="leaderboard-title-note"]').text())
    const anonymous = squash(
      mountTitle({ participantCount: '100+' }).find('[data-testid="leaderboard-title-note"]').text(),
    )

    for (const note of [named, anonymous]) {
      expect(note).not.toContain('mode')
      expect(note).not.toContain('costs')
      expect(note).not.toContain('email')
      expect(note).not.toContain('user ID')
    }
  })

  // 「正在计算」MUST NOT 渲染成 0 值：人数那一段整段略过。
  it('drops the participant segment while the snapshot is not ready', () => {
    const note = squash(
      mountTitle({ ready: false, participantCount: 0 })
        .find('[data-testid="leaderboard-title-note"]')
        .text(),
    )

    expect(note).not.toContain('0 active')
    expect(note).not.toContain('active')
    expect(note).toBe('2026-09-12')
  })

  // anonymous 档的人数是分档字符串（如 `100+`），照原样印出来。
  it('prints a banded participant count verbatim', () => {
    const note = squash(
      mountTitle({ participantCount: '100+' })
        .find('[data-testid="leaderboard-title-note"]')
        .text(),
    )

    expect(note).toBe('100+ active ·2026-09-12')
  })

  // 快照还没生成时日期退回当天，而不是留空或印一个假日期。
  it('falls back to today when there is no snapshot yet', () => {
    const note = squash(
      mountTitle({ snapshotUpdatedAt: null, ready: false })
        .find('[data-testid="leaderboard-title-note"]')
        .text(),
    )

    expect(note).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })
})
