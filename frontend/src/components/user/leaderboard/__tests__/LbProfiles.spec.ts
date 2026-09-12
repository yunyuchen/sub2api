import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbProfiles from '../LbProfiles.vue'
import type { LeaderboardProfile } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.profiles.note': 'Model mix · top 3',
  'leaderboard.profiles.more': 'others omitted',
  'leaderboard.identity.anonymous': 'Row {ordinal}',
  'leaderboard.identity.outOfRank': 'Outside top 50',
  'channelMonitorV2.currentUser': 'Current user',
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

/**
 * 展示名 helper（`displayName.ts`）会读 auth store 里的 username：本人行优先显示自己的
 * 昵称，没有昵称时才回退「当前用户」。默认置空以覆盖回退分支，需要昵称的用例自己赋值。
 */
const authState = vi.hoisted(() => ({ user: null as { username: string } | null }))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authState }))

beforeEach(() => {
  authState.user = null
})

/** named 档：显示昵称的用户给出用户名，关掉昵称展示的仍是 Ordinal 假名「第 N 位」。 */
function namedProfiles(): LeaderboardProfile[] {
  return [
    {
      identity: { kind: 'named', username: 'alice' },
      ordinal: 1,
      models: [
        { model: 'claude-sonnet-5', share_percent: 62 },
        { model: 'claude-opus-5', share_percent: 25 },
        { model: 'gpt-5.1', share_percent: 13 },
      ],
    },
    {
      identity: { kind: 'anonymous' },
      ordinal: 3,
      models: [{ model: 'gpt-5.1', share_percent: 71 }],
    },
  ]
}

/** anonymous 档：所有人都是假名，占比两档都下发，整块本来就没有绝对量。 */
function anonymousProfiles(): LeaderboardProfile[] {
  return namedProfiles().map((profile, index) => ({
    ...profile,
    identity: { kind: 'anonymous' as const },
    ordinal: index + 1,
  }))
}

function mountProfiles(profiles: LeaderboardProfile[] | null) {
  return mount(LbProfiles, { props: { profiles } })
}

describe('LbProfiles', () => {
  // 画板里 Top 3 之和不足 100% 的那一行补一枚灰字 chip，MUST NOT 把它读成「只用这三个」。
  it('adds an "others omitted" chip only when the top three fall well short of 100%', () => {
    const short = mountProfiles([
      {
        identity: { kind: 'anonymous' },
        ordinal: 4,
        models: [
          { model: 'haiku-4-5', share_percent: 20 },
          { model: 'opus-5', share_percent: 20 },
          { model: 'sonnet-5', share_percent: 20 },
        ],
      },
    ])
    expect(short.find('[data-testid="leaderboard-profiles-more"]').text()).toBe('others omitted')
    // 正经的 chip 数不受影响
    expect(short.findAll('[data-testid="leaderboard-profiles-chip"]')).toHaveLength(3)

    // 取整残差（33 + 33 + 33 = 99）不算「还有别的模型」
    const rounded = mountProfiles([
      {
        identity: { kind: 'anonymous' },
        ordinal: 5,
        models: [
          { model: 'a', share_percent: 33 },
          { model: 'b', share_percent: 33 },
          { model: 'c', share_percent: 33 },
        ],
      },
    ])
    expect(rounded.find('[data-testid="leaderboard-profiles-more"]').exists()).toBe(false)

    // 只有一两个模型时后端已经给全了，不可能有第 4 个
    const single = mountProfiles([
      { identity: { kind: 'anonymous' }, ordinal: 6, models: [{ model: 'a', share_percent: 71 }] },
    ])
    expect(single.find('[data-testid="leaderboard-profiles-more"]').exists()).toBe(false)
  })

  it('renders nothing when the block is absent or empty', () => {
    expect(mountProfiles(null).find('[data-testid="leaderboard-profiles"]').exists()).toBe(false)
    expect(mountProfiles([]).find('[data-testid="leaderboard-profiles"]').exists()).toBe(false)
  })

  it('renders the eyebrow and one row per user in named mode', () => {
    const wrapper = mountProfiles(namedProfiles())

    expect(wrapper.find('.rp-eyebrow').text()).toBe('Model mix · top 3')

    const rows = wrapper.findAll('[data-testid="leaderboard-profiles-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].find('.rp-u').text()).toBe('alice')
    expect(rows[0].find('.rp-u').classes()).not.toContain('is-self')

    // chip 的左边界对齐成一条轴：每行的 chip 都在同一个 `.rp-cs` 容器里
    const chips = rows[0].findAll('[data-testid="leaderboard-profiles-chip"]')
    expect(chips).toHaveLength(3)
    expect(chips[0].text()).toBe('claude-sonnet-562%')
    expect(rows[0].find('.rp-cs').exists()).toBe(true)
  })

  // 假名与「榜外用户」同榜单条目一套规则，展示名一律由前端渲染。
  it('renders ordinal pseudonyms in anonymous mode and never a username', () => {
    const wrapper = mountProfiles(anonymousProfiles())
    const rows = wrapper.findAll('[data-testid="leaderboard-profiles-row"]')

    expect(rows[0].find('.rp-u').text()).toBe('Row 1')
    expect(rows[1].find('.rp-u').text()).toBe('Row 2')
    expect(wrapper.text()).not.toContain('alice')
    // 占比两档都下发
    expect(wrapper.text()).toContain('62%')
  })

  it('falls back to the out-of-rank wording and marks the viewer', () => {
    const wrapper = mountProfiles([
      {
        identity: { kind: 'anonymous' },
        ordinal: null,
        models: [{ model: 'grok-4', share_percent: 80 }],
      },
      {
        identity: { kind: 'self' },
        ordinal: 12,
        models: [{ model: 'kimi-k2', share_percent: 40 }],
      },
    ])

    const rows = wrapper.findAll('[data-testid="leaderboard-profiles-row"]')
    expect(rows[0].find('.rp-u').text()).toBe('Outside top 50')
    expect(rows[0].find('.rp-u').classes()).not.toContain('is-self')
    // 本人那一行用强调色，与榜单里的本人行是同一条线索
    expect(rows[1].find('.rp-u').text()).toBe('Current user')
    expect(rows[1].find('.rp-u').classes()).toContain('is-self')
  })

  // 本人那一行优先显示自己的昵称，「当前用户」只是没有昵称时的兜底。
  it('names the viewer row after the signed-in username', () => {
    authState.user = { username: 'zoe' }
    const wrapper = mountProfiles([
      {
        identity: { kind: 'self' },
        ordinal: 4,
        models: [{ model: 'kimi-k2', share_percent: 40 }],
      },
    ])

    const row = wrapper.find('[data-testid="leaderboard-profiles-row"]')
    expect(row.find('.rp-u').text()).toBe('zoe')
    expect(row.find('.rp-u').classes()).toContain('is-self')
  })

  it('keeps at most eight rows', () => {
    const many = Array.from({ length: 11 }, (_, index) => ({
      identity: { kind: 'anonymous' as const },
      ordinal: index + 1,
      models: [{ model: `model-${index}`, share_percent: 100 - index }],
    }))

    expect(mountProfiles(many).findAll('[data-testid="leaderboard-profiles-row"]')).toHaveLength(8)
  })
})
