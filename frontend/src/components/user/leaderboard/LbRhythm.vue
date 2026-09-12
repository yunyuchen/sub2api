<template>
  <!-- 06 左：7 × 24 的周内节奏。等级是相对近 4 周最大值的 0–4 档，两档完全相同
       （没有绝对量可漏）。整块为 null（来源整表缺行 / 旧后端）时不渲染；
       个别格子为 0 是「那个时段没有请求」，不是缺数据。 -->
  <div v-if="rows.length" class="rp-r" style="--i: 1" data-testid="leaderboard-rhythm">
    <span class="rp-eyebrow">{{ t('leaderboard.rhythm.note') }}</span>

    <div class="rp-week-grid">
      <template v-for="(row, index) in rows" :key="row.key">
        <span class="rp-week-lbl">{{ row.label }}</span>
        <span class="rp-week-row rp-growy" :style="{ '--i': index }">
          <i
            v-for="cell in row.cells"
            :key="cell.hour"
            :class="cell.levelClass"
            :title="cell.title"
            data-testid="leaderboard-rhythm-cell"
          ></i>
        </span>
      </template>
    </div>

    <!-- 小时刻度与格子同一套轨道，因此照抄一行空的行标列让它对齐 -->
    <div class="rp-week-grid" style="margin-top: 2px" aria-hidden="true">
      <span></span>
      <span class="rp-hourscale">
        <span v-for="hour in HOURS" :key="hour">{{ (hour - 1) % 3 === 0 ? hour - 1 : '' }}</span>
      </span>
    </div>

    <div class="rp-scale-key">
      {{ t('leaderboard.insights.heatmap.legendLow') }}
      <i></i><i class="rp-lv1"></i><i class="rp-lv2"></i><i class="rp-lv3"></i><i class="rp-lv4"></i>
      {{ t('leaderboard.insights.heatmap.legendHigh') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const props = defineProps<{
  /** `insights.weekly_rhythm`，7 × 24 的 0–4 等级，行序周一起；为 null 或空时整块不渲染。 */
  rhythm: number[][] | null
}>()

/** 行序固定周一起，与后端的 ISODOW 一致。 */
const WEEKDAY_KEYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as const
const HOURS_PER_DAY = 24
/** 模板里 `v-for="hour in HOURS"` 是 1..24，取小时值时要减 1。 */
const HOURS = HOURS_PER_DAY

interface RhythmCell {
  hour: number
  levelClass: string
  title: string
}

interface RhythmRow {
  key: string
  label: string
  cells: RhythmCell[]
}

const rows = computed<RhythmRow[]>(() => {
  const source = props.rhythm ?? []
  if (source.length === 0) return []

  return source.slice(0, WEEKDAY_KEYS.length).map((hours, index) => {
    const key = WEEKDAY_KEYS[index]
    const label = t(`leaderboard.rhythm.weekdays.${key}`)
    const cells = Array.from({ length: HOURS_PER_DAY }, (_, hour) => {
      // 来源短了就按 0 档补齐：格子的 0 表示「那个时段没有请求」。
      const level = clampLevel(hours?.[hour])
      return {
        hour,
        // 0 档不加类，直接用格子的底色，与近 30 天热力图同一套色阶。
        levelClass: level > 0 ? `rp-lv${level}` : '',
        title: `${label} ${paddedHour(hour)}:00`,
      }
    })
    return { key, label, cells }
  })
})

function clampLevel(value: number | undefined): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0
  return Math.max(0, Math.min(4, Math.round(value)))
}

function paddedHour(hour: number): string {
  return `${hour}`.padStart(2, '0')
}
</script>
