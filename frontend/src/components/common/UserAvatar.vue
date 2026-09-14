<template>
  <div :class="containerClass">
    <img
      v-if="showImage"
      :src="resolvedUrl"
      alt=""
      class="h-full w-full rounded-full object-cover"
      referrerpolicy="no-referrer"
      loading="lazy"
      data-test="user-avatar-image"
      @error="handleImageError"
    >
    <span v-else :class="initialClass" data-test="user-avatar-initial">{{ initial }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

/**
 * 通用用户头像：有 avatar_url（inline 时是 data URL，外链时是 http(s) URL）就显示真实头像，
 * 没有或加载失败时回退成 `name` 的首字母圆圈。
 *
 * `name` 只是首字母的来源，调用方决定传什么：后台用户列表传邮箱，榜单传展示名。
 * 传空串时圆圈里不渲染任何字符、只留底色——榜单的匿名行就用这一形态（design D25）：
 * 那一行本来就不该露出任何可辨识的字符，一个素色空圆既占住位置又不泄露身份。
 */
const props = withDefaults(
  defineProps<{
    /** 首字母的来源（邮箱 / 展示名）；空串表示「不显示任何字符」。 */
    name: string
    avatarUrl?: string | null
    size?: 'xs' | 'sm' | 'md' | 'lg'
  }>(),
  {
    avatarUrl: null,
    size: 'sm'
  }
)

// 图片加载失败标记；avatarUrl 变化时重置，让新地址有机会重新加载
const failed = ref(false)

const resolvedUrl = computed(() => props.avatarUrl?.trim() || '')
const showImage = computed(() => resolvedUrl.value !== '' && !failed.value)
// 取第一个码点而不是 charAt(0)：展示名可能以 emoji 等非 BMP 字符开头，charAt 会切出半个代理对。
const initial = computed(() => (Array.from(props.name || '')[0] ?? '').toUpperCase())

const sizeClass = computed(() => {
  if (props.size === 'lg') return 'h-14 w-14'
  if (props.size === 'md') return 'h-10 w-10'
  if (props.size === 'xs') return 'h-6 w-6'
  return 'h-8 w-8'
})

const initialSizeClass = computed(() => {
  if (props.size === 'lg') return 'text-2xl'
  if (props.size === 'md') return 'text-lg'
  if (props.size === 'xs') return 'text-[10px]'
  return 'text-sm'
})

const containerClass = computed(() => [
  'flex flex-shrink-0 items-center justify-center overflow-hidden rounded-full',
  sizeClass.value,
  showImage.value ? '' : 'bg-primary-100 dark:bg-primary-900/30'
])

const initialClass = computed(() => [
  'font-medium text-primary-700 dark:text-primary-300',
  initialSizeClass.value
])

watch(resolvedUrl, () => {
  failed.value = false
})

const handleImageError = () => {
  failed.value = true
}
</script>
