import { onUnmounted, ref, watch, type Ref } from 'vue'

/**
 * 数字滚动累加（排行榜独立页的 Highlights 与洞察卡在用）。
 *
 * rAF + easeOutExpo，默认 600ms，缓动曲线与样式里的 cubic-bezier(0.16, 1, 0.3, 1) 对齐。
 * `prefers-reduced-motion: reduce` 时直接跳终值且不排任何 rAF —— 这是 design D16 的
 * 「脚本层直接跳终值」那一条，样式层的媒体查询只管 CSS 动画，管不到这里。
 */

/** cubic-bezier(0.16, 1, 0.3, 1) 的近似缓动。 */
function easeOutExpo(t: number): number {
  return t === 1 ? 1 : 1 - Math.pow(2, -10 * t)
}

function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

export interface UseCountUpOptions {
  /** 动画时长（毫秒），默认 600。 */
  duration?: number
}

/**
 * @param targetRef 目标数字（响应式，computed 也可以）
 * @returns `current` 为正在滚动的中间值，模板自行决定取整或保留小数
 */
export function useCountUp(
  targetRef: Ref<number>,
  options: UseCountUpOptions = {}
): { current: Ref<number> } {
  const duration = options.duration ?? 600
  const current = ref(Number(targetRef.value) || 0)
  let rafId: number | null = null
  let startTime = 0
  let startValue = 0
  let endValue = current.value

  function tick(now: number) {
    const elapsed = now - startTime
    const t = Math.min(1, elapsed / duration)
    current.value = startValue + (endValue - startValue) * easeOutExpo(t)
    if (t < 1) {
      rafId = requestAnimationFrame(tick)
    } else {
      current.value = endValue
      rafId = null
    }
  }

  function cancel() {
    if (rafId !== null) {
      cancelAnimationFrame(rafId)
      rafId = null
    }
  }

  watch(targetRef, (next) => {
    const value = Number(next) || 0
    if (value === endValue) return
    if (prefersReducedMotion()) {
      cancel()
      startValue = value
      endValue = value
      current.value = value
      return
    }
    startValue = current.value
    endValue = value
    startTime = typeof performance !== 'undefined' ? performance.now() : Date.now()
    cancel()
    rafId = requestAnimationFrame(tick)
  })

  onUnmounted(cancel)

  return { current }
}
