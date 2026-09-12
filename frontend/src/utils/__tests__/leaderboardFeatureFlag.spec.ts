import { beforeEach, describe, expect, it, vi } from 'vitest'

// leaderboard_mode 是枚举而非布尔，不能登记进 FeatureFlags 注册表
// （isFeatureFlagEnabled 只认 typeof raw === 'boolean'，枚举会恒为 false）。
// 这里直接断言枚举读取器与由它派生的布尔函数的行为。
const appStore = vi.hoisted(() => ({
  cachedPublicSettings: null as Record<string, unknown> | null,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

import { getLeaderboardMode, isLeaderboardVisible } from '@/utils/featureFlags'

function withSettings(settings: Record<string, unknown> | null) {
  appStore.cachedPublicSettings = settings
}

describe('getLeaderboardMode', () => {
  beforeEach(() => {
    withSettings(null)
  })

  it('公开设置未加载时按 off 处理', () => {
    withSettings(null)
    expect(getLeaderboardMode()).toBe('off')
  })

  it('缺少 leaderboard_mode 键时按 off 处理', () => {
    withSettings({ channel_monitor_mode: 'v2' })
    expect(getLeaderboardMode()).toBe('off')
  })

  it('识别三档合法值', () => {
    withSettings({ leaderboard_mode: 'off' })
    expect(getLeaderboardMode()).toBe('off')
    withSettings({ leaderboard_mode: 'anonymous' })
    expect(getLeaderboardMode()).toBe('anonymous')
    withSettings({ leaderboard_mode: 'named' })
    expect(getLeaderboardMode()).toBe('named')
  })

  it.each([
    ['非法字符串', 'public'],
    ['空字符串', ''],
    ['大小写变体', 'Named'],
    ['带空格', ' named '],
    ['布尔值', true],
    ['数字', 1],
    ['null', null],
  ])('%s 一律归一化为 off（fail-closed）', (_label, raw) => {
    withSettings({ leaderboard_mode: raw })
    expect(getLeaderboardMode()).toBe('off')
  })
})

describe('isLeaderboardVisible', () => {
  beforeEach(() => {
    withSettings(null)
  })

  it.each([
    ['off', 'off', false],
    ['anonymous', 'anonymous', true],
    ['named', 'named', true],
  ])('%s 档下的可见性', (_label, mode, expected) => {
    withSettings({ leaderboard_mode: mode })
    expect(isLeaderboardVisible()).toBe(expected)
  })

  it('设置未加载时隐藏入口（枚举无 opt-out 宽容语义）', () => {
    withSettings(null)
    expect(isLeaderboardVisible()).toBe(false)
  })
})
