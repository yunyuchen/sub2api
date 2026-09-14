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
 * 后台用户头像：有 avatar_url（inline 时是 data URL，外链时是 http(s) URL）就显示真实头像，
 * 没有或加载失败时回退成邮箱首字母圆圈。
 */
const props = withDefaults(
  defineProps<{
    email: string
    avatarUrl?: string | null
    size?: 'sm' | 'md' | 'lg'
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
const initial = computed(() => (props.email || '').charAt(0).toUpperCase())

const containerClass = computed(() => [
  'flex flex-shrink-0 items-center justify-center overflow-hidden rounded-full',
  props.size === 'lg' ? 'h-14 w-14' : props.size === 'md' ? 'h-10 w-10' : 'h-8 w-8',
  showImage.value ? '' : 'bg-primary-100 dark:bg-primary-900/30'
])

const initialClass = computed(() => [
  'font-medium text-primary-700 dark:text-primary-300',
  props.size === 'lg' ? 'text-2xl' : props.size === 'md' ? 'text-lg' : 'text-sm'
])

watch(resolvedUrl, () => {
  failed.value = false
})

const handleImageError = () => {
  failed.value = true
}
</script>
