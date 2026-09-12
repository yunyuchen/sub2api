<template>
  <!-- Preview：仅 off 档的管理员能走到这里，普通用户拿到的是 404。横幅在标题块之上。 -->
  <div
    v-if="variant === 'preview'"
    class="rp-banner"
    role="status"
    data-testid="leaderboard-preview-banner"
  >
    <div class="rp-banner-title">{{ t('leaderboard.preview.banner') }}</div>
    <p class="rp-hint">{{ t('leaderboard.preview.bannerHint') }}</p>
  </div>

  <!-- 陈旧：数据照常展示，只是不能静默。 -->
  <div
    v-else-if="variant === 'stale'"
    class="rp-banner"
    role="status"
    aria-live="polite"
    data-testid="leaderboard-stale"
  >
    <div class="rp-banner-title">{{ t('leaderboard.states.stale') }}</div>
    <p class="rp-hint">{{ t('leaderboard.states.staleHint') }}</p>
  </div>

  <!-- 正在计算：Snapshot 尚未生成，主榜是骨架而不是空卡，也不是报错。
       骨架就是几条 `--bg-sunk` 色块，MUST NOT 用圆形 spinner。 -->
  <div v-else-if="variant === 'computing'" data-testid="leaderboard-computing">
    <div v-for="row in rows" :key="row" class="rp-skel"></div>
    <p class="rp-hint">
      {{ t('leaderboard.states.computing') }} · {{ t('leaderboard.states.computingHint') }}
    </p>
  </div>

  <!-- 抑制态：由 entries_suppressed 判定，不从「entries 是空数组」反推；只剩「你的位置」。
       原因那一句归 whoami 那一格（myRank.suppressedHint），这里只留结论，不说两遍。 -->
  <div v-else class="rp-empty" data-testid="leaderboard-suppressed">
    <p>{{ t('leaderboard.states.suppressed') }}</p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

withDefaults(
  defineProps<{
    /** 四种非正常态，判定只用 preview / status / stale / entries_suppressed 四个字段。 */
    variant: 'preview' | 'stale' | 'computing' | 'suppressed'
    /** computing 骨架的行数。 */
    rows?: number
  }>(),
  { rows: 4 },
)
</script>
