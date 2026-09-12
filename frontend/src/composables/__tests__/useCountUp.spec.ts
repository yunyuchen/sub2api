import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, ref, type Ref } from 'vue'
import { mount } from '@vue/test-utils'

import { useCountUp } from '@/composables/useCountUp'

/** 按查询精确作答的 matchMedia，测试环境默认的那一个对所有查询都返回 true。 */
function stubMatchMedia(reduce: boolean) {
  window.matchMedia = ((query: string) => ({
    matches: query.includes('prefers-reduced-motion') ? reduce : false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia
}

function mountCountUp(target: Ref<number>) {
  const component = defineComponent({
    setup() {
      const { current } = useCountUp(target)
      return () => h('span', String(current.value))
    },
  })
  return mount(component)
}

describe('useCountUp', () => {
  const originalMatchMedia = window.matchMedia
  const originalRaf = globalThis.requestAnimationFrame

  beforeEach(() => {
    vi.restoreAllMocks()
  })

  afterEach(() => {
    window.matchMedia = originalMatchMedia
    globalThis.requestAnimationFrame = originalRaf
  })

  it('jumps straight to the final value and schedules no rAF under reduced motion', async () => {
    stubMatchMedia(true)
    const raf = vi.fn()
    globalThis.requestAnimationFrame = raf as unknown as typeof requestAnimationFrame

    const target = ref(0)
    const wrapper = mountCountUp(target)

    target.value = 4200
    await nextTick()

    expect(wrapper.text()).toBe('4200')
    expect(raf).not.toHaveBeenCalled()
  })

  it('animates through rAF when motion is allowed and lands exactly on the target', async () => {
    stubMatchMedia(false)
    const frames: FrameRequestCallback[] = []
    globalThis.requestAnimationFrame = ((callback: FrameRequestCallback) => {
      frames.push(callback)
      return frames.length
    }) as unknown as typeof requestAnimationFrame

    const target = ref(0)
    const wrapper = mountCountUp(target)

    target.value = 1000
    await nextTick()
    expect(frames).toHaveLength(1)

    // 半程：已经在走，但还没到终值
    frames[0](performance.now() + 300)
    await nextTick()
    expect(Number(wrapper.text())).toBeGreaterThan(0)
    expect(Number(wrapper.text())).toBeLessThan(1000)

    // 超过 duration 的那一帧必须收敛到终值，而不是停在缓动的近似值
    frames[frames.length - 1](performance.now() + 5000)
    await nextTick()
    expect(wrapper.text()).toBe('1000')
  })

  it('cancels the pending frame when the component unmounts', async () => {
    stubMatchMedia(false)
    const cancel = vi.fn()
    globalThis.requestAnimationFrame = (() => 7) as unknown as typeof requestAnimationFrame
    globalThis.cancelAnimationFrame = cancel as unknown as typeof cancelAnimationFrame

    const target = ref(0)
    const wrapper = mountCountUp(target)
    target.value = 12
    await nextTick()

    wrapper.unmount()
    expect(cancel).toHaveBeenCalledWith(7)
  })
})
