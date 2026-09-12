import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbPlatforms from '../LbPlatforms.vue'
import type { LeaderboardPlatform } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.platforms.note': 'Today requests by platform',
  'leaderboard.platforms.legendCount': '{count} requests',
  'leaderboard.platforms.other': 'Other',
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

/** named 档：图例带成功请求数（与榜单同口径）。四项相加正好 100，没有残差。 */
function namedPlatforms(): LeaderboardPlatform[] {
  return [
    { platform: 'anthropic', successful_requests: 1979, share_percent: 58 },
    { platform: 'openai', successful_requests: 921, share_percent: 27 },
    { platform: 'gemini', successful_requests: 307, share_percent: 9 },
    { platform: 'other', successful_requests: 205, share_percent: 6 },
  ]
}

/** anonymous 档：站点级绝对量一律缺席，只剩占比。 */
function anonymousPlatforms(): LeaderboardPlatform[] {
  return namedPlatforms().map(({ platform, share_percent }) => ({ platform, share_percent }))
}

function mountPlatforms(platforms: LeaderboardPlatform[] | null) {
  return mount(LbPlatforms, { props: { platforms } })
}

describe('LbPlatforms', () => {
  it('renders nothing when the block is absent or empty', () => {
    expect(mountPlatforms(null).find('[data-testid="leaderboard-platforms"]').exists()).toBe(false)
    expect(mountPlatforms([]).find('[data-testid="leaderboard-platforms"]').exists()).toBe(false)
  })

  it('draws one stacked segment per platform, sized by the shares', () => {
    const wrapper = mountPlatforms(namedPlatforms())

    expect(wrapper.find('.rp-eyebrow').text()).toBe('Today requests by platform')

    const segments = wrapper.findAll('[data-testid="leaderboard-platforms-segment"]')
    expect(segments).toHaveLength(4)
    expect(segments[0].attributes('style')).toContain('flex: 58 0 0')
    // 同一个色相的四级明度，不换色相
    expect(segments[0].classes()).toContain('rp-sw1')
    expect(segments[3].classes()).toContain('rp-sw4')
  })

  it('puts the request counts in the legend in named mode', () => {
    const legend = mountPlatforms(namedPlatforms()).findAll(
      '[data-testid="leaderboard-platforms-legend"]',
    )

    expect(legend[0].text()).toContain('anthropic')
    expect(legend[0].text()).toContain('58%')
    expect(legend[0].text()).toContain('1,979 requests')
  })

  // 「档位决定字段是否存在」：anonymous 档一个绝对量都不能出现。
  it('drops every count in anonymous mode and keeps the shares', () => {
    const wrapper = mountPlatforms(anonymousPlatforms())
    const legend = wrapper.findAll('[data-testid="leaderboard-platforms-legend"]')

    expect(legend[0].text()).toContain('anthropic')
    expect(legend[0].text()).toContain('58%')
    expect(legend[0].text()).not.toContain('requests')
    expect(wrapper.text()).not.toContain('1,979')
    expect(wrapper.findAll('[data-testid="leaderboard-platforms-segment"]')).toHaveLength(4)
  })

  // 残差项只在各项占比之和不足 100 时出现，且 MUST NOT 为它编一个请求数，
  // 也不再挂一句「取整残差」的解释——图例上一个 `Other 6%` 已经说完了。
  it('adds the rounding remainder only when the shares fall short of 100', () => {
    const exact = mountPlatforms(namedPlatforms())
    expect(exact.text()).not.toContain('Other')

    const short = mountPlatforms([
      { platform: 'anthropic', successful_requests: 1979, share_percent: 58 },
      { platform: 'openai', successful_requests: 921, share_percent: 27 },
      { platform: 'gemini', successful_requests: 307, share_percent: 9 },
    ])
    const legend = short.findAll('[data-testid="leaderboard-platforms-legend"]')
    expect(legend).toHaveLength(4)
    expect(legend[3].text()).toContain('Other')
    expect(legend[3].text()).toBe('Other6%')
    expect(legend[3].text()).not.toContain('remainder')
    expect(legend[3].text()).not.toContain('requests')
    // 条上也多一段，宽度就是那 6%
    const segments = short.findAll('[data-testid="leaderboard-platforms-segment"]')
    expect(segments).toHaveLength(4)
    expect(segments[3].attributes('style')).toContain('flex: 6 0 0')
  })

  it('never adds a remainder when the shares already overflow', () => {
    const wrapper = mountPlatforms([
      { platform: 'anthropic', share_percent: 61 },
      { platform: 'openai', share_percent: 40 },
    ])

    expect(wrapper.findAll('[data-testid="leaderboard-platforms-legend"]')).toHaveLength(2)
    expect(wrapper.text()).not.toContain('Other')
  })

  it('cycles the four tones when more platforms arrive', () => {
    const many = Array.from({ length: 5 }, (_, index) => ({
      platform: `platform-${index}`,
      share_percent: 20,
    }))

    const segments = mountPlatforms(many).findAll('[data-testid="leaderboard-platforms-segment"]')
    expect(segments).toHaveLength(5)
    expect(segments[4].classes()).toContain('rp-sw1')
  })
})
