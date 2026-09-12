import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import LbModelHeat from '../LbModelHeat.vue'
import type { LeaderboardModelUsage } from '@/api/leaderboard'

const messages: Record<string, string> = {
  'leaderboard.insights.modelHeat.title': 'Models today',
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

/** named 档：计数带成功落账过滤，与榜单同口径。 */
function namedModels(): LeaderboardModelUsage[] {
  return [
    { model: 'claude-sonnet-5', successful_requests: 1624, share_percent: 48 },
    { model: 'claude-opus-5', successful_requests: 802, share_percent: 24 },
    { model: 'gpt-5.1', successful_requests: 410, share_percent: 12 },
  ]
}

/** anonymous 档：站点级绝对量一个都不下发，只剩占比。 */
function anonymousModels(): LeaderboardModelUsage[] {
  return namedModels().map(({ model, share_percent }) => ({ model, share_percent }))
}

function mountModelHeat(models: LeaderboardModelUsage[] | null) {
  return mount(LbModelHeat, { props: { models } })
}

describe('LbModelHeat', () => {
  it('renders nothing when the block is absent or empty', () => {
    expect(mountModelHeat(null).find('[data-testid="leaderboard-model-heat"]').exists()).toBe(false)
    expect(mountModelHeat([]).find('[data-testid="leaderboard-model-heat"]').exists()).toBe(false)
  })

  it('shows the model name, the request count and the share in named mode', () => {
    const wrapper = mountModelHeat(namedModels())

    expect(wrapper.find('.rp-eyebrow').text()).toBe('Models today')

    const rows = wrapper.findAll('[data-testid="leaderboard-model-heat-row"]')
    expect(rows).toHaveLength(3)
    expect(rows[0].text()).toContain('claude-sonnet-5')
    expect(rows[0].find('.rp-c').text()).toBe('1,624')
    expect(rows[0].find('.rp-p').text()).toBe('48%')
  })

  // anonymous 档下一个绝对量都不出现，条长仍按占比相对第一名。
  it('leaves the count cell empty in anonymous mode and keeps the shares', () => {
    const wrapper = mountModelHeat(anonymousModels())

    const rows = wrapper.findAll('[data-testid="leaderboard-model-heat-row"]')
    expect(rows[0].find('.rp-c').text()).toBe('')
    expect(rows[0].find('.rp-p').text()).toBe('48%')
    expect(wrapper.text()).not.toContain('1,624')
    expect(rows[1].find('.rp-p').text()).toBe('24%')
  })

  it('scales the bars against the leading model in both modes', () => {
    const named = mountModelHeat(namedModels()).findAll('.rp-bar-fill')
    expect(named[0].attributes('style')).toContain('width: 100%')
    expect(named[1].attributes('style')).toContain('width: 50%')

    const anonymous = mountModelHeat(anonymousModels()).findAll('.rp-bar-fill')
    expect(anonymous[0].attributes('style')).toContain('width: 100%')
    expect(anonymous[1].attributes('style')).toContain('width: 50%')
  })

  it('keeps at most the top 8 models', () => {
    const many = Array.from({ length: 12 }, (_, index) => ({
      model: `model-${index}`,
      successful_requests: 100 - index,
      share_percent: 20 - index,
    }))

    expect(mountModelHeat(many).findAll('[data-testid="leaderboard-model-heat-row"]')).toHaveLength(
      8,
    )
  })
})
