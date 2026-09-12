<template>
  <!-- 页脚是一行 colophon，四段：快照时分、重建周期、站点时区、隐私声明。
       `COLOPHON` 字样、`一周从周一起算` 与 `successful_requests = actual_cost > 0`
       已按「只留数字与必要标签」删掉；预聚合口径那句话本来就不在这里。
       `snapshot` 与时区名是技术字面量，不进 i18n（design D4）。 -->
  <footer class="rp-colophon" data-testid="leaderboard-snapshot-meta">
    <div class="rp-line">
      <span>snapshot <b>{{ snapshotText }}</b></span>
      <span aria-hidden="true">·</span>
      <span>{{ t('leaderboard.footer.rebuild') }}</span>
      <span aria-hidden="true">·</span>
      <span><b>{{ timezone }}</b></span>
      <span aria-hidden="true">·</span>
      <span>{{ t('leaderboard.footer.noMoney') }}</span>
    </div>
  </footer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatTimeToMinuteInTimeZone } from '@/utils/format'

const { t } = useI18n()

const props = defineProps<{
  /** 窗口边界所用的站点时区名，常驻展示。 */
  timezone: string
  /** Snapshot 尚未生成时为 null，此处换成 `pending` 而不是空白。 */
  snapshotUpdatedAt: string | null
}>()

/**
 * 时分按站点时区渲染：同一行里已经写着时区名，用浏览器本地时区会与它自相矛盾（design D4）。
 */
const snapshotText = computed(() => {
  if (!props.snapshotUpdatedAt) return 'pending'
  return formatTimeToMinuteInTimeZone(props.snapshotUpdatedAt, props.timezone) || 'pending'
})
</script>
