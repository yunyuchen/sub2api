import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbComposition from '../LbComposition.vue'
import type { LeaderboardComposition } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.composition.note': 'Today token mix across the four kinds',
  'leaderboard.composition.input': 'Input',
  'leaderboard.composition.output': 'Output',
  'leaderboard.composition.cacheCreation': 'Cache creation',
  'leaderboard.composition.cacheRead': 'Cache read',
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

/** named 档：四个绝对量齐备。 */
function namedComposition(): LeaderboardComposition {
  return {
    input_tokens: 8_800_000,
    output_tokens: 2_600_000,
    cache_creation_tokens: 1_900_000,
    cache_read_tokens: 8_100_000,
    input_percent: 41,
    output_percent: 12,
    cache_creation_percent: 9,
    cache_read_percent: 38,
  }
}

/** anonymous 档：四段占比照给，四个绝对量一律缺席。 */
function anonymousComposition(): LeaderboardComposition {
  const {
    input_percent,
    output_percent,
    cache_creation_percent,
    cache_read_percent,
  } = namedComposition()
  return { input_percent, output_percent, cache_creation_percent, cache_read_percent }
}

function mountComposition(composition: LeaderboardComposition | null) {
  return mount(LbComposition, { props: { composition } })
}

describe('LbComposition', () => {
  it('renders nothing when the block is absent', () => {
    expect(mountComposition(null).find('[data-testid="leaderboard-composition"]').exists()).toBe(
      false,
    )
  })

  it('draws the four segments in a fixed order, sized by their shares', () => {
    const wrapper = mountComposition(namedComposition())

    expect(wrapper.find('.rp-eyebrow').text()).toBe('Today token mix across the four kinds')

    const segments = wrapper.findAll('[data-testid="leaderboard-composition-segment"]')
    expect(segments).toHaveLength(4)
    expect(segments[0].attributes('style')).toContain('flex: 41 0 0')
    // 同一个色相的四级明度，不换色相
    expect(segments[0].classes()).toContain('rp-sw1')
    expect(segments[1].classes()).toContain('rp-sw2')
    expect(segments[2].classes()).toContain('rp-sw3')
    expect(segments[3].classes()).toContain('rp-sw4')
  })

  it('puts the absolute token counts in the legend in named mode', () => {
    const legend = mountComposition(namedComposition()).findAll(
      '[data-testid="leaderboard-composition-legend"]',
    )

    expect(legend[0].text()).toContain('Input')
    expect(legend[0].text()).toContain('41%')
    expect(legend[0].text()).toContain('8.8M')
    expect(legend[3].text()).toContain('Cache read')
    expect(legend[3].text()).toContain('8.1M')
  })

  // anonymous 档没有站点级绝对量：图例只剩百分比。
  it('drops every absolute value in anonymous mode', () => {
    const wrapper = mountComposition(anonymousComposition())
    const legend = wrapper.findAll('[data-testid="leaderboard-composition-legend"]')

    expect(legend[0].text()).toContain('Input')
    expect(legend[0].text()).toContain('41%')
    expect(legend[1].text()).toContain('12%')
    expect(legend[0].find('em').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('8.8M')
    expect(wrapper.text()).not.toContain('8.1M')
  })
})
