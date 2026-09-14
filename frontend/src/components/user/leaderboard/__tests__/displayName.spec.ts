import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useLeaderboardDisplayName } from '../displayName'

const messages: Record<string, string> = {
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

const authState = vi.hoisted(() => ({
  user: null as { username?: string; avatar_url?: string | null } | null,
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => authState }))

beforeEach(() => {
  authState.user = null
})

/**
 * 头像（design D25）与展示名走同一套身份规则，所以 `avatarUrl` 与 `displayName` 同住一处：
 * named 才可能有，self 从自己的个人资料取，anonymous 恒无。
 */
describe('useLeaderboardDisplayName().avatarUrl', () => {
  it('takes the viewer avatar from the signed-in profile', () => {
    authState.user = { username: 'zoe', avatar_url: 'data:image/webp;base64,BBBB' }
    const { avatarUrl } = useLeaderboardDisplayName()

    expect(avatarUrl({ identity: { kind: 'self' }, ordinal: 4 })).toBe(
      'data:image/webp;base64,BBBB',
    )
  })

  it.each([
    { label: 'absent', user: { username: 'zoe' } },
    { label: 'null', user: { username: 'zoe', avatar_url: null } },
    { label: 'blank', user: { username: 'zoe', avatar_url: '   ' } },
    { label: 'signed out', user: null },
  ])('returns an empty string when the viewer avatar is $label', ({ user }) => {
    authState.user = user
    const { avatarUrl } = useLeaderboardDisplayName()

    expect(avatarUrl({ identity: { kind: 'self' }, ordinal: 4 })).toBe('')
  })

  it('uses the backend thumbnail for a named holder', () => {
    const { avatarUrl } = useLeaderboardDisplayName()

    expect(
      avatarUrl({
        identity: { kind: 'named', username: 'alice', avatar_url: 'data:image/jpeg;base64,AAAA' },
        ordinal: 1,
      }),
    ).toBe('data:image/jpeg;base64,AAAA')
  })

  // 外链头像（remote_url）没有小图，后端不下发：这里返回空串，组件回退首字母。
  it('returns an empty string for a named holder without a thumbnail', () => {
    const { avatarUrl } = useLeaderboardDisplayName()

    expect(avatarUrl({ identity: { kind: 'named', username: 'alice' }, ordinal: 1 })).toBe('')
  })

  // 匿名行 MUST NOT 拿到任何头像：一张脸就够把「第 N 位」认回来。
  it('never returns an avatar for an anonymous holder', () => {
    authState.user = { username: 'zoe', avatar_url: 'data:image/webp;base64,BBBB' }
    const { avatarUrl } = useLeaderboardDisplayName()

    expect(avatarUrl({ identity: { kind: 'anonymous' }, ordinal: 7 })).toBe('')
    expect(avatarUrl({ identity: { kind: 'anonymous' }, ordinal: null })).toBe('')
  })
})

describe('useLeaderboardDisplayName().displayName', () => {
  it('renders each identity form without ever exposing a user id', () => {
    authState.user = { username: 'zoe' }
    const { displayName } = useLeaderboardDisplayName()

    expect(displayName({ identity: { kind: 'self' }, ordinal: 4 })).toBe('zoe')
    expect(displayName({ identity: { kind: 'named', username: 'alice' }, ordinal: 1 })).toBe('alice')
    expect(displayName({ identity: { kind: 'anonymous' }, ordinal: 7 })).toBe('Row 7')
    expect(displayName({ identity: { kind: 'anonymous' }, ordinal: null })).toBe('Outside top 50')
  })
})
