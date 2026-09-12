<template>
  <div :class="props.embedded ? 'space-y-4' : 'card'">
    <div
      v-if="!props.embedded"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.editProfile') }}
      </h2>
    </div>
    <div :class="props.embedded ? '' : 'px-6 py-6'">
      <form @submit.prevent="handleUpdateProfile" class="space-y-4">
        <div v-if="props.embedded">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('profile.editProfile') }}
          </p>
        </div>
        <div>
          <label for="username" class="input-label">
            {{ t('profile.username') }}
          </label>
          <input
            id="username"
            v-model="username"
            type="text"
            class="input"
            :placeholder="t('profile.enterUsername')"
          />
        </div>

        <!-- 昵称展示：默认开启，表示同意在排行榜上以 username 显示；
             关闭后仍然参与排名，只是显示成「第 N 位」 -->
        <div
          data-testid="profile-leaderboard-named-participation"
          class="flex items-start justify-between gap-4 border-t border-gray-100 pt-4 dark:border-dark-700"
        >
          <div class="min-w-0">
            <p class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('profile.leaderboardNamedParticipation') }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('profile.leaderboardNamedParticipationHint') }}
            </p>
            <p
              v-if="namedParticipationError"
              data-testid="profile-leaderboard-named-participation-error"
              class="mt-1 text-xs text-red-600 dark:text-red-400"
            >
              {{ namedParticipationError }}
            </p>
          </div>
          <Toggle
            :model-value="leaderboardNamedParticipation"
            @update:modelValue="onNamedParticipationToggle"
          />
        </div>

        <div class="flex justify-end pt-4">
          <button type="submit" :disabled="loading" class="btn btn-primary">
            {{ loading ? t('profile.updating') : t('profile.updateProfile') }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'
import { extractApiErrorMessage } from '@/utils/apiError'
import Toggle from '@/components/common/Toggle.vue'

const props = withDefaults(defineProps<{
  initialUsername: string
  initialLeaderboardNamedParticipation?: boolean
  embedded?: boolean
}>(), {
  // 后端默认开启（既有用户已回填 true）；这里的兜底值只在父组件没传时用到，
  // 跟后端默认保持一致，MUST NOT 写死 false。
  initialLeaderboardNamedParticipation: true,
  embedded: false,
})

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const username = ref(props.initialUsername)
const leaderboardNamedParticipation = ref(props.initialLeaderboardNamedParticipation)
// 后端对 username 的昵称展示校验（长度 / 字符集 / 邮箱形态 / 保留词）与个人资料的通用
// username 校验是两套规则；拒绝时后端给的 message 是英文，双语文案由 reason
// （LEADERBOARD_USERNAME_INVALID）映射到本地化提示，见 handleUpdateProfile。
const namedParticipationError = ref('')
const loading = ref(false)

watch(() => props.initialUsername, (val) => {
  username.value = val
})

watch(() => props.initialLeaderboardNamedParticipation, (val) => {
  leaderboardNamedParticipation.value = val
})

// 清空拒绝提示只跟「用户又动了一次开关」走，不能写成 watch(leaderboardNamedParticipation)：
// 请求失败时开关会被回滚到上次持久化的值，那次回滚会触发 watch，把同一个 catch 里刚
// 钉上去的拒绝提示又擦掉，结果是提示永远看不见。
function onNamedParticipationToggle(value: boolean) {
  leaderboardNamedParticipation.value = value
  namedParticipationError.value = ''
}

const handleUpdateProfile = async () => {
  if (!username.value.trim()) {
    appStore.showError(t('profile.usernameRequired'))
    return
  }

  loading.value = true
  namedParticipationError.value = ''
  // 开关没动过就不提交这个字段（后端是指针语义，缺省即不动）。否则「昵称展示已开启的
  // 用户只改 username」也会被后端拿新 username 重跑一次展示校验，改成一个合法但不够
  // 展示资格的名字时整次 PUT 会被 400 挡掉，连改名一起丢。开启后把 username 改坏的场景
  // 由渲染时的二次校验兜底：条目回退成「第 N 位」，而不是把改名这条路堵死。
  const namedParticipationChanged =
    leaderboardNamedParticipation.value !== props.initialLeaderboardNamedParticipation
  const requestedEnable = namedParticipationChanged && leaderboardNamedParticipation.value
  try {
    const updatedUser = await userAPI.updateProfile({
      username: username.value,
      ...(namedParticipationChanged
        ? { leaderboard_named_participation: leaderboardNamedParticipation.value }
        : {})
    })
    authStore.user = updatedUser
    // 响应里字段缺席（旧后端）按后端默认的「开启」读，别把缺席塌成关闭。
    leaderboardNamedParticipation.value = updatedUser.leaderboard_named_participation !== false
    appStore.showSuccess(t('profile.updateSuccess'))
  } catch (error: unknown) {
    // 后端用 reason 区分错误：昵称展示被拒（LEADERBOARD_USERNAME_INVALID）有专门的
    // 双语文案，其余沿用后端 message。直接读 message 会让中文界面吃到英文原句。
    const message = extractApiErrorMessage(error, t('profile.updateFailed'), {
      LEADERBOARD_USERNAME_INVALID: t('profile.leaderboardNamedParticipationRejected')
    })
    // 整个表单是一次 PUT，失败即什么都没落库，开关回退到上次持久化的值，
    // 免得界面显示「已开启」而实际没生效。
    leaderboardNamedParticipation.value = props.initialLeaderboardNamedParticipation
    // 只有这次确实尝试开启实名时，才把拒绝理由钉在开关旁边。
    if (requestedEnable) {
      namedParticipationError.value = message
    }
    appStore.showError(message)
  } finally {
    loading.value = false
  }
}
</script>
