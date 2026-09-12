import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbExtremes from '../LbExtremes.vue'
import type { LeaderboardExtremes } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.extremes.nightOwl.label': 'Night owl',
  'leaderboard.extremes.nightOwl.unit': 'of their own tokens between 0:00 and 6:00',
  'leaderboard.extremes.rising.label': 'Rising star',
  'leaderboard.extremes.rising.unit': 'vs yesterday',
  'leaderboard.extremes.omnivore.label': 'Omnivore',
  'leaderboard.extremes.omnivore.unit': 'different models',
  'leaderboard.extremes.talker.label': 'Talker',
  'leaderboard.extremes.talker.unit': 'output token share',
  'leaderboard.extremes.maxSingle.label': 'Largest single call',
  'leaderboard.extremes.maxSingle.unit': 'tokens per request',
  'leaderboard.extremes.maxSingle.ratioUnit': '× the median peak',
  'leaderboard.extremes.maxSingle.medianNote': '{ratio}× the median',
  'leaderboard.extremes.streak.label': 'Longest streak',
  'leaderboard.extremes.streak.unit': 'days in a row',
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

function namedExtremes(): LeaderboardExtremes {
  return {
    night_owl: {
      identity: { kind: 'named', username: 'grace' },
      ordinal: 9,
      night_tokens: 620_000,
      night_share_percent: 62,
    },
    rising: { identity: { kind: 'named', username: 'dave' }, ordinal: 7, change_percent: 240 },
    omnivore: { identity: { kind: 'anonymous' }, ordinal: 3, distinct_models: 6 },
    talker: {
      identity: { kind: 'named', username: 'alice' },
      ordinal: 1,
      output_share_percent: 71,
    },
    max_single: {
      identity: { kind: 'anonymous' },
      ordinal: 5,
      max_single_tokens: 1_200_000,
      ratio_to_median: 3.2,
    },
    streak: { identity: { kind: 'named', username: 'erin' }, ordinal: 2, days: 23 },
  }
}

/** anonymous 档：他人绝对量一个都不下发，只剩占比与相对值。 */
function anonymousExtremes(): LeaderboardExtremes {
  const named = namedExtremes()
  return {
    ...named,
    night_owl: { identity: { kind: 'anonymous' }, ordinal: 9, night_share_percent: 62 },
    rising: null,
    max_single: { identity: { kind: 'anonymous' }, ordinal: 5, ratio_to_median: 3.2 },
  }
}

function mountExtremes(extremes: LeaderboardExtremes | null) {
  return mount(LbExtremes, { props: { extremes } })
}

/** 大数字与单位是相邻节点，断言前把空白折成一个空格。 */
function squash(text: string): string {
  return text.replace(/\s+/g, ' ').trim()
}

describe('LbExtremes', () => {
  it('renders one cell per dimension with the value and its unit on one line', () => {
    const wrapper = mountExtremes(namedExtremes())

    expect(wrapper.find('[data-testid="leaderboard-extremes"]').exists()).toBe(true)
    expect(wrapper.findAll('.rp-sup > li')).toHaveLength(6)

    const nightOwl = wrapper.find('[data-testid="leaderboard-extreme-night-owl"]')
    expect(nightOwl.text()).toContain('grace')
    expect(squash(nightOwl.find('.rp-sup-v').text())).toBe(
      '62%of their own tokens between 0:00 and 6:00',
    )
    // 夜猫子两档同口径，只给占比：绝对量放不进这一格（画板的实名档同样只画占比）。
    expect(nightOwl.text()).not.toContain('620K')

    expect(squash(wrapper.find('[data-testid="leaderboard-extreme-rising"]').text())).toContain(
      '+240%vs yesterday',
    )
    expect(squash(wrapper.find('[data-testid="leaderboard-extreme-omnivore"]').text())).toContain(
      '6different models',
    )
    expect(squash(wrapper.find('[data-testid="leaderboard-extreme-talker"]').text())).toContain(
      '71%output token share',
    )
    expect(squash(wrapper.find('[data-testid="leaderboard-extreme-max-single"]').text())).toContain(
      '1.2Mtokens per request',
    )
    expect(squash(wrapper.find('[data-testid="leaderboard-extreme-streak"]').text())).toContain(
      '23days in a row',
    )
  })

  // 「只在 today 窗口」那句注脚已删：进步之星本来就只在 today 窗口才有格子，缺席即是说明。
  it('drops the window-scope footnote from the rising star', () => {
    const rising = mountExtremes(namedExtremes()).find('[data-testid="leaderboard-extreme-rising"]')

    expect(rising.text()).not.toContain('window only')
    expect(rising.find('.rp-sup-who em').exists()).toBe(false)
  })

  // 实名档的单次最大同时给绝对量与「中位数的几倍」；匿名档只剩倍数。
  it('falls back to the median multiple in anonymous mode', () => {
    const named = mountExtremes(namedExtremes())
    expect(named.find('[data-testid="leaderboard-extreme-max-single"]').text()).toContain(
      '3.2× the median',
    )

    const wrapper = mountExtremes(anonymousExtremes())
    const maxSingle = wrapper.find('[data-testid="leaderboard-extreme-max-single"]')
    expect(squash(maxSingle.find('.rp-sup-v').text())).toBe('3.2× the median peak')
    expect(maxSingle.text()).not.toContain('1.2M')

    const nightOwl = wrapper.find('[data-testid="leaderboard-extreme-night-owl"]')
    expect(nightOwl.text()).toContain('Row 9')
    expect(nightOwl.text()).not.toContain('620K')
  })

  // 阶梯位移由 `--i` 现算：缺席的项不在序列里留洞，后面的项自动前移。
  it('renumbers the stagger index when a dimension is missing', () => {
    const wrapper = mountExtremes(anonymousExtremes())

    const items = wrapper.findAll('.rp-sup > li')
    expect(items).toHaveLength(5)
    expect(items[0].attributes('style')).toContain('--i: 0')
    expect(items[1].attributes('data-testid')).toBe('leaderboard-extreme-omnivore')
    expect(items[1].attributes('style')).toContain('--i: 1')
  })

  // rising 只在 today 窗口存在，week / month 下必然缺席：隐藏该格而不是渲染一个空格。
  it('hides a single cell whose dimension is null and keeps the others', () => {
    const wrapper = mountExtremes(anonymousExtremes())

    expect(wrapper.find('[data-testid="leaderboard-extreme-rising"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="leaderboard-extreme-streak"]').exists()).toBe(true)
  })

  it('renders nothing at all when the whole block is absent', () => {
    expect(mountExtremes(null).find('[data-testid="leaderboard-extremes"]').exists()).toBe(false)
  })

  it('renders nothing when every dimension is empty', () => {
    const empty: LeaderboardExtremes = {
      night_owl: null,
      rising: null,
      omnivore: null,
      talker: null,
      max_single: null,
      streak: null,
    }

    expect(mountExtremes(empty).find('[data-testid="leaderboard-extremes"]').exists()).toBe(false)
  })

  // 领先者不在前 50 时没有 Ordinal 可用，渲染成「榜外用户」而不是编一个号。
  it('names an out-of-board holder without inventing an ordinal', () => {
    const extremes = namedExtremes()
    extremes.streak = { identity: { kind: 'anonymous' }, ordinal: null, days: 31 }
    const wrapper = mountExtremes(extremes)

    expect(wrapper.find('[data-testid="leaderboard-extreme-streak"]').text()).toContain(
      'Outside top 50',
    )
  })

  it('names the viewer as the current user when they hold a record', () => {
    const extremes = namedExtremes()
    extremes.talker = { identity: { kind: 'self' }, ordinal: 4, output_share_percent: 71 }
    const wrapper = mountExtremes(extremes)

    expect(wrapper.find('[data-testid="leaderboard-extreme-talker"]').text()).toContain(
      'Current user',
    )
  })

  // 本人持有的之最卡也优先显示自己的昵称，「当前用户」只是没有昵称时的兜底。
  it('names the viewer after the signed-in username when they hold a record', () => {
    authState.user = { username: 'zoe' }
    const extremes = namedExtremes()
    extremes.talker = { identity: { kind: 'self' }, ordinal: 4, output_share_percent: 71 }
    const wrapper = mountExtremes(extremes)

    const talker = wrapper.find('[data-testid="leaderboard-extreme-talker"]').text()
    expect(talker).toContain('zoe')
    expect(talker).not.toContain('Current user')
  })
})
