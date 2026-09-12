<template>
  <!-- 标题块：H1 一句随 Window 换，副题只剩「N 位活跃 · 日期」。
       档位说明与「不展示金额、邮箱与用户 ID」那一句已删——前者在匿名档由报头的 chip 表达，
       后者页脚已经写了一遍，同一句话 MUST NOT 在页面上出现两次。
       参与人数还没算出来时整段略过它：「正在计算」MUST NOT 渲染成 0 值。 -->
  <section class="rp-titleblock" data-testid="leaderboard-title">
    <div class="rp-r" style="--i: 1">
      <h1 class="rp-h1" data-testid="leaderboard-title-heading">{{ headingText }}</h1>
      <p class="rp-sub" data-testid="leaderboard-title-note">
        <template v-if="ready">
          <span class="rp-n">{{ participantLabel }}</span>
          {{ t('leaderboard.titleBlock.participantsUnit') }}
          <span class="rp-dotsep">·</span>
        </template>
        <span class="rp-n">{{ dateText }}</span>
      </p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatDateOnlyInTimeZone, formatNumberLocaleString } from '@/utils/format'
import type { LeaderboardWindow } from '@/api/leaderboard'

const { t } = useI18n()

const props = defineProps<{
  activeWindow: LeaderboardWindow
  /**
   * 快照已就绪、响应已到手。为 false 时（加载中或 status = computing）参与人数还没算出来，
   * 副题整段略过它。
   */
  ready: boolean
  /** named 档与 Preview 下是精确整数，anonymous 档下是分档字符串（如 `100+`）。 */
  participantCount: number | string
  /** 快照时间，缺席时日期退回当天。 */
  snapshotUpdatedAt: string | null
  /** 窗口边界所用的站点时区名，日期按它渲染而不是浏览器本地时区。 */
  timezone: string
}>()

/** H1 随 Window 换一句：今天 / 本周 / 本月「谁在用」。窗口字面量已经在报头的分段上，这里不回显。 */
const headingText = computed(() => t(`leaderboard.titleBlock.heading.${props.activeWindow}`))

const participantLabel = computed(() =>
  typeof props.participantCount === 'number'
    ? formatNumberLocaleString(props.participantCount)
    : props.participantCount,
)

/**
 * 日期固定成 YYYY-MM-DD。按站点时区渲染：这一行说的是「哪一天的榜」，
 * 跨时区查看者看到的必须与页脚的 tz 一致。
 */
const dateText = computed(() =>
  formatDateOnlyInTimeZone(props.snapshotUpdatedAt ?? new Date(), props.timezone),
)
</script>
