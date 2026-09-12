import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

/**
 * 榜单假名与昵称展示开关的文案守门。
 *
 * 「用户 #N」这种写法会被读成用户编号，而 N 其实只是榜单序号（ordinal），所以整条产品线
 * 统一改成「第 N 位」/「Row N」。这里把措辞钉死，免得日后有人顺手改回编号口吻，
 * 也顺带确认个人资料的开关是「默认开、可关掉」的说法，而不是旧的「自选实名」。
 */
describe('leaderboard identity locales', () => {
  it('names other people by leaderboard ordinal, never by a user number', () => {
    expect(zh.leaderboard.identity.anonymous).toBe('第 {ordinal} 位')
    expect(en.leaderboard.identity.anonymous).toBe('Row {ordinal}')
  })

  it('keeps the user-number wording out of every leaderboard message', () => {
    for (const [locale, messages] of Object.entries({ en, zh })) {
      const offenders = Object.entries(flatten(messages.leaderboard))
        .filter(([, value]) => /用户\s*#/.test(value) || /\bUser\s*#/.test(value))
        .map(([key]) => key)
      expect(offenders, `${locale} leaderboard copy still uses a user number`).toEqual([])
    }
  })

  it('frames the profile toggle as showing a nickname that is on by default', () => {
    expect(zh.profile.leaderboardNamedParticipation).toBe('在排行榜显示我的昵称')
    expect(zh.profile.leaderboardNamedParticipationHint).toContain('默认开启')
    expect(zh.profile.leaderboardNamedParticipationHint).toContain('第 N 位')
    expect(zh.profile.leaderboardNamedParticipationHint).not.toContain('实名')

    expect(en.profile.leaderboardNamedParticipation).toBe('Show my name on the leaderboard')
    expect(en.profile.leaderboardNamedParticipationHint).toContain('On by default')
    expect(en.profile.leaderboardNamedParticipationHint).toContain('Row N')
  })

  it('describes the named mode as opt-out on the admin settings page', () => {
    expect(zh.admin.settings.features.leaderboard.modeNamedHint).toContain('默认显示昵称')
    expect(zh.admin.settings.features.leaderboard.modeNamedHint).toContain('可自行关闭')
    expect(zh.admin.settings.features.leaderboard.modeNamedHint).not.toContain('自选实名')
    expect(en.admin.settings.features.leaderboard.modeNamedHint).toContain('by default')
  })
})

function flatten(value: unknown, prefix = ''): Record<string, string> {
  if (typeof value === 'string') {
    return prefix ? { [prefix]: value } : {}
  }
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return {}
  }
  return Object.entries(value as Record<string, unknown>).reduce<Record<string, string>>(
    (acc, [key, child]) => Object.assign(acc, flatten(child, prefix ? `${prefix}.${key}` : key)),
    {}
  )
}
