<template>
  <!-- 内联 SVG 折线：名次走势与缓存命中率趋势共用。没有点时整块不渲染，
       MUST NOT 画一条代表 0 的平线——那会把「没有数据」画成「一直是 0」。
       线宽、填充与端点由 `.rp-chart` 的样式给；`color` 走行内样式，因此仍然压得过样式表。 -->
  <svg
    v-if="polyline"
    ref="svgEl"
    class="rp-chart"
    :viewBox="`0 0 ${viewW} ${height}`"
    preserveAspectRatio="none"
    :style="{ height: `${height}px` }"
    :role="label ? 'img' : undefined"
    :aria-label="label"
    :aria-hidden="label ? undefined : 'true'"
    data-testid="leaderboard-sparkline"
  >
    <path v-if="fill" class="rp-chart-area" :d="areaPath" :style="{ fill: color }"></path>
    <path
      v-if="smoothPath"
      class="rp-chart-line rp-chart-line-smooth"
      :d="smoothPath"
      :style="{ stroke: color }"
    ></path>
    <polyline
      class="rp-chart-line rp-chart-line-data"
      :points="polyline"
      :style="{ stroke: color }"
      aria-hidden="true"
    ></polyline>
    <!-- 一天一枚点：末点是实心大点，其余是空心小点（画板 chart_line 如此）。 -->
    <circle
      v-for="dot in dots"
      :key="dot.key"
      class="rp-chart-dot"
      :class="dot.last ? 'is-last' : ''"
      :cx="dot.x"
      :cy="dot.y"
      :r="dot.r"
      :style="dot.last ? { fill: color } : { stroke: color }"
    ></circle>
  </svg>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    /** 折线的值，按时间升序。空数组时整块不渲染。 */
    points: number[]
    /** 值越小画得越高（名次走势用）。 */
    invert?: boolean
    /** 描边与填充色，默认主色；传 CSS 变量即可跟随主题。 */
    color?: string
    /** 折线下方的淡色面积。 */
    fill?: boolean
    /** 视口高度（px），宽度总是撑满容器。 */
    height?: number
    /** 有值时整块作为 img 暴露给读屏，没有则按装饰处理。 */
    label?: string
  }>(),
  {
    invert: false,
    color: 'var(--accent)',
    fill: true,
    height: 56,
    label: undefined,
  },
)

/**
 * viewBox 宽度跟随 svg 的实际渲染宽度：坐标按真实像素算，preserveAspectRatio="none"
 * 就不会做非等比拉伸——否则圆点会被横向拉成椭圆、曲线的横竖线宽也不一致。
 * 量不到宽度（jsdom / 尚未挂载）时退回名义宽度 300。
 */
const FALLBACK_W = 300
const svgEl = ref<SVGSVGElement | null>(null)
const viewW = ref(FALLBACK_W)
let observer: ResizeObserver | null = null

onMounted(() => {
  if (typeof ResizeObserver === 'undefined' || !svgEl.value) return
  observer = new ResizeObserver((entries) => {
    const width = entries[0]?.contentRect.width ?? 0
    viewW.value = width >= 1 ? Math.round(width) : FALLBACK_W
  })
  observer.observe(svgEl.value)
})
onBeforeUnmount(() => {
  observer?.disconnect()
  observer = null
})
const PAD_X = 4
const PAD_Y = 6

interface Point {
  x: number
  y: number
}

const coords = computed<Point[]>(() => {
  const values = props.points.filter((value) => Number.isFinite(value))
  if (values.length === 0) return []

  const low = Math.min(...values)
  const high = Math.max(...values)
  // 名次趋势使用固定的上界，避免 1→4 这类小范围变化被 min/max 拉满，
  // 造成视觉上夸张的上下跳动；其它趋势仍保持数据自适应。
  const domainLow = props.invert ? 1 : low
  const domainHigh = props.invert ? Math.max(high, 6) : high
  // 全平的一段（含单点）没有极差可分，统一画在中线上
  const span = domainHigh - domainLow
  const innerH = props.height - PAD_Y * 2

  const innerW = viewW.value - PAD_X * 2

  return values.map((value, index) => {
    const ratio = span === 0 ? 0.5 : (value - domainLow) / span
    const x = values.length === 1 ? viewW.value / 2 : PAD_X + (index * innerW) / (values.length - 1)
    const y = props.invert ? PAD_Y + ratio * innerH : props.height - PAD_Y - ratio * innerH
    return { x: round(x), y: round(y) }
  })
})
/**
 * Catmull-Rom 转三次贝塞尔：曲线经过每一个数据点且切线连续，没有折角；
 * 控制点钳制在数据的 y 范围内，陡变处（名次 1↔4）不会过冲出数据范围。
 */
const smoothPath = computed(() => {
  const points = coords.value
  if (points.length < 2) return ''
  if (points.length === 2) {
    return `M ${points[0].x} ${points[0].y} L ${points[1].x} ${points[1].y}`
  }

  const minY = Math.min(...points.map((point) => point.y))
  const maxY = Math.max(...points.map((point) => point.y))
  const clampY = (value: number): number => round(Math.min(maxY, Math.max(minY, value)))

  let d = `M ${points[0].x} ${points[0].y}`
  for (let index = 0; index < points.length - 1; index += 1) {
    const p0 = points[index - 1] ?? points[index]
    const p1 = points[index]
    const p2 = points[index + 1]
    const p3 = points[index + 2] ?? p2
    const cp1x = round(p1.x + (p2.x - p0.x) / 6)
    const cp1y = clampY(p1.y + (p2.y - p0.y) / 6)
    const cp2x = round(p2.x - (p3.x - p1.x) / 6)
    const cp2y = clampY(p2.y - (p3.y - p1.y) / 6)
    d += ` C ${cp1x} ${cp1y} ${cp2x} ${cp2y} ${p2.x} ${p2.y}`
  }
  return d
})

/** 面积与线共用同一条平滑路径，闭合到底边；MUST NOT 再用直线顶点拼面积。 */
const areaPath = computed(() => {
  const d = smoothPath.value
  if (!d) return ''
  return `${d} L ${viewW.value - PAD_X} ${props.height} L ${PAD_X} ${props.height} Z`
})
const polyline = computed(() => coords.value.map((point) => `${point.x},${point.y}`).join(' '))


/** 末点单独加粗成实心点，其余各点是空心小点；半径与填充都由这里决定。 */
const dots = computed(() =>
  coords.value.map((point, index) => ({
    key: index,
    x: point.x,
    y: point.y,
    last: index === coords.value.length - 1,
    r: index === coords.value.length - 1 ? 2.6 : 1.8,
  })),
)

function round(value: number): number {
  return Math.round(value * 10) / 10
}
</script>
