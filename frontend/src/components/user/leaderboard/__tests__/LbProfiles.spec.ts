import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbProfiles from '../LbProfiles.vue'
import type { LeaderboardProfile } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.profiles.note': 'Model mix · share',
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
  // 每人列出该窗口用过的全部模型，按后端给的顺序，不截断也不补「其余略」。
  it('renders every model the backend sends, in order', () => {
    const wrapper = mountProfiles([
      {
        identity: { kind: 'anonymous' },
        ordinal: 4,
        models: [
          { model: 'haiku-4-5', share_percent: 40 },
          { model: 'opus-5', share_percent: 30 },
          { model: 'sonnet-5', share_percent: 20 },
          { model: 'gpt-5.1', share_percent: 7 },
          { model: 'gemini-3', share_percent: 3 },
        ],
      },
    ])
    const chips = wrapper.findAll('[data-testid="leaderboard-profiles-chip"]')
    expect(chips.map((chip) => chip.text())).toEqual([
      'haiku-4-540%',
      'opus-530%',
      'sonnet-520%',
      'gpt-5.17%',
      'gemini-33%',
    ])
    expect(wrapper.find('[data-testid="leaderboard-profiles-more"]').exists()).toBe(false)
  })

  // 后端把占比取整成整数，不足 0.5% 会变成 0：这种模型确实被调过，MUST NOT 显示成 0%。
  it('shows sub-percent shares as <1% instead of 0%', () => {
    const wrapper = mountProfiles([
      {
        identity: { kind: 'anonymous' },
        ordinal: 5,
        models: [
          { model: 'gpt-6-astra', share_percent: 100 },
          { model: 'gpt-5.6-luna', share_percent: 0 },
        ],
      },
    ])
    const chips = wrapper.findAll('[data-testid="leaderboard-profiles-chip"]')
    expect(chips[1].text()).toBe('gpt-5.6-luna<1%')
  })

  it('renders nothing when the block is absent or empty', () => {
    expect(mountProfiles(null).find('[data-testid="leaderboard-profiles"]').exists()).toBe(false)
    expect(mountProfiles([]).find('[data-testid="leaderboard-profiles"]').exists()).toBe(false)
  })

  it('renders the eyebrow and one row per user in named mode', () => {
    const wrapper = mountProfiles(namedProfiles())

    expect(wrapper.find('.rp-eyebrow').text()).toBe('Model mix · share')

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

  // 画像与榜单本体同为 Top 50：后端已按 Total Tokens 取前 50 名，这里只兜一层底。
  // 多出来的行 MUST NOT 渲染，少于 50 行时有多少渲染多少，不补位。
  it('keeps at most fifty rows and never pads', () => {
    const many = Array.from({ length: 60 }, (_, index) => ({
      identity: { kind: 'anonymous' as const },
      ordinal: index + 1,
      models: [{ model: `model-${index}`, share_percent: 100 - index }],
    }))

    expect(mountProfiles(many).findAll('[data-testid="leaderboard-profiles-row"]')).toHaveLength(50)
    expect(mountProfiles(many.slice(0, 12)).findAll('[data-testid="leaderboard-profiles-row"]')).toHaveLength(12)
  })

  // 50 行逐行递延会让末行等到 ~3s 才入场；与榜单同样按 4 行一档级联，末行不超过 ~0.7s。
  it('cascades the reveal four rows per step like the rank list', () => {
    const many = Array.from({ length: 9 }, (_, index) => ({
      identity: { kind: 'anonymous' as const },
      ordinal: index + 1,
      models: [{ model: `model-${index}`, share_percent: 90 }],
    }))
    const rows = mountProfiles(many).findAll('[data-testid="leaderboard-profiles-row"]')
    const steps = rows.map((row) => (row.element as HTMLElement).style.getPropertyValue('--i'))
    expect(steps).toEqual(['0', '0', '0', '0', '1', '1', '1', '1', '2'])
  })
})
