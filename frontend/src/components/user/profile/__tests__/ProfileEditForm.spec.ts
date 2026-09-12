import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfileEditForm from '@/components/user/profile/ProfileEditForm.vue'

const updateProfile = vi.hoisted(() => vi.fn())
const authStore = vi.hoisted(() => ({ user: null as unknown }))
const appStore = vi.hoisted(() => ({ showError: vi.fn(), showSuccess: vi.fn() }))

vi.mock('@/api', () => ({
  userAPI: { updateProfile }
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore
}))

// 文案断言只看键名：真正要盯的是「走没走 i18n」，而不是某个句子的措辞。
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

// Toggle 的最小替身：点一下就翻转 v-model。
vi.mock('@/components/common/Toggle.vue', () => ({
  default: {
    name: 'Toggle',
    props: { modelValue: { type: Boolean, default: false } },
    emits: ['update:modelValue'],
    template:
      '<button type="button" data-testid="toggle" @click="$emit(\'update:modelValue\', !modelValue)"></button>'
  }
}))

function mountForm(initialLeaderboardNamedParticipation = false) {
  return mount(ProfileEditForm, {
    props: {
      initialUsername: 'alice',
      initialLeaderboardNamedParticipation
    }
  })
}

/** 父组件没传开关时走组件自己的兜底值，用来盯住「兜底跟后端默认一致」。 */
function mountFormWithoutToggleProp() {
  return mount(ProfileEditForm, { props: { initialUsername: 'alice' } })
}

async function submit(wrapper: ReturnType<typeof mountForm>) {
  await wrapper.find('form').trigger('submit.prevent')
  await flushPromises()
}

describe('ProfileEditForm named participation', () => {
  beforeEach(() => {
    updateProfile.mockReset()
    appStore.showError.mockReset()
    appStore.showSuccess.mockReset()
    updateProfile.mockResolvedValue({ username: 'bob', leaderboard_named_participation: true })
  })

  // 开关没动过就不提交该字段：否则「昵称展示已开启的用户只改 username」会被后端拿新
  // username 重跑一次展示校验，改成合法但不够展示资格的名字时整次 PUT 被 400 挡掉。
  it('omits leaderboard_named_participation when the toggle is untouched', async () => {
    const wrapper = mountForm(true)

    await wrapper.find('#username').setValue('bob')
    await submit(wrapper)

    expect(updateProfile).toHaveBeenCalledWith({ username: 'bob' })
  })

  // 昵称展示后端默认开启（既有用户已回填 true），前端兜底值 MUST NOT 写死 false，
  // 否则新用户一进个人资料就看到「已关闭」，而服务端其实是开着的。
  it('defaults the toggle to on when the parent passes no value', () => {
    const wrapper = mountFormWithoutToggleProp()

    expect(wrapper.findComponent({ name: 'Toggle' }).props('modelValue')).toBe(true)
  })

  // 响应里字段缺席（旧后端）按后端默认的「开启」读，别把缺席塌成关闭。
  it('keeps the toggle on when the response omits the field', async () => {
    updateProfile.mockResolvedValue({ username: 'alice' })
    const wrapper = mountForm(true)

    await wrapper.find('#username').setValue('bob')
    await submit(wrapper)

    expect(wrapper.findComponent({ name: 'Toggle' }).props('modelValue')).toBe(true)
  })

  it('sends the toggle value once it actually changes', async () => {
    const wrapper = mountForm(false)

    await wrapper.find('[data-testid="toggle"]').trigger('click')
    await submit(wrapper)

    expect(updateProfile).toHaveBeenCalledWith({
      username: 'alice',
      leaderboard_named_participation: true
    })
  })

  it('sends false when the toggle is turned off', async () => {
    updateProfile.mockResolvedValue({ username: 'alice', leaderboard_named_participation: false })
    const wrapper = mountForm(true)

    await wrapper.find('[data-testid="toggle"]').trigger('click')
    await submit(wrapper)

    expect(updateProfile).toHaveBeenCalledWith({
      username: 'alice',
      leaderboard_named_participation: false
    })
  })

  // 拒绝理由按 reason 映射到本地化文案：后端 message 是英文原句，直接展示会让
  // 中文界面吃到英文。
  it('localizes the rejection by reason instead of echoing the backend message', async () => {
    updateProfile.mockRejectedValue({
      status: 400,
      code: 400,
      reason: 'LEADERBOARD_USERNAME_INVALID',
      message: 'username is not eligible for named participation on the leaderboard'
    })
    const wrapper = mountForm(false)

    await wrapper.find('[data-testid="toggle"]').trigger('click')
    await submit(wrapper)

    const error = wrapper.get('[data-testid="profile-leaderboard-named-participation-error"]')
    expect(error.text()).toBe('profile.leaderboardNamedParticipationRejected')
    expect(error.text()).not.toContain('username is not eligible')
    expect(appStore.showError).toHaveBeenCalledWith('profile.leaderboardNamedParticipationRejected')
    // 失败即什么都没落库，开关回到上次持久化的值；回滚 MUST NOT 顺手擦掉刚钉上的提示。
    expect(wrapper.findComponent({ name: 'Toggle' }).props('modelValue')).toBe(false)
  })

  it('keeps other backend errors off the toggle row', async () => {
    updateProfile.mockRejectedValue({ status: 409, code: 409, message: 'username already exists' })
    const wrapper = mountForm(false)

    await wrapper.find('#username').setValue('bob')
    await submit(wrapper)

    expect(wrapper.find('[data-testid="profile-leaderboard-named-participation-error"]').exists()).toBe(false)
    expect(appStore.showError).toHaveBeenCalledWith('username already exists')
  })
})
