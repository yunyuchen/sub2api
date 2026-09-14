import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import UserAvatar from '../UserAvatar.vue'

const DATA_URL = 'data:image/webp;base64,AAAA'
const HTTP_URL = 'https://cdn.example.com/avatar.png'

describe('admin UserAvatar', () => {
  it('renders the avatar image when an avatar URL is provided', () => {
    const wrapper = mount(UserAvatar, {
      props: { email: 'scoped@example.com', avatarUrl: DATA_URL }
    })

    const img = wrapper.get('[data-test="user-avatar-image"]')
    expect(img.attributes('src')).toBe(DATA_URL)
    expect(img.attributes('alt')).toBe('')
    expect(img.attributes('referrerpolicy')).toBe('no-referrer')
    expect(img.attributes('loading')).toBe('lazy')
    expect(wrapper.find('[data-test="user-avatar-initial"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('falls back to the email initial when there is no avatar URL', () => {
    const wrapper = mount(UserAvatar, {
      props: { email: 'scoped@example.com' }
    })

    expect(wrapper.find('[data-test="user-avatar-image"]').exists()).toBe(false)
    const initial = wrapper.get('[data-test="user-avatar-initial"]')
    expect(initial.text()).toBe('S')
    expect(initial.classes()).toContain('text-primary-700')
    expect(initial.classes()).toContain('dark:text-primary-300')
    expect(wrapper.classes()).toContain('bg-primary-100')
    expect(wrapper.classes()).toContain('dark:bg-primary-900/30')
    wrapper.unmount()
  })

  it.each([
    { avatarUrl: null, label: 'null' },
    { avatarUrl: '   ', label: 'blank' }
  ])('treats a $label avatar URL as no avatar', ({ avatarUrl }) => {
    const wrapper = mount(UserAvatar, {
      props: { email: 'blank@example.com', avatarUrl }
    })

    expect(wrapper.find('[data-test="user-avatar-image"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="user-avatar-initial"]').text()).toBe('B')
    wrapper.unmount()
  })

  it('falls back to the initial after the image fails to load', async () => {
    const wrapper = mount(UserAvatar, {
      props: { email: 'scoped@example.com', avatarUrl: HTTP_URL }
    })

    await wrapper.get('[data-test="user-avatar-image"]').trigger('error')

    expect(wrapper.find('[data-test="user-avatar-image"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="user-avatar-initial"]').text()).toBe('S')
    wrapper.unmount()
  })

  it('retries with a new image after the avatar URL changes', async () => {
    const wrapper = mount(UserAvatar, {
      props: { email: 'scoped@example.com', avatarUrl: HTTP_URL }
    })

    await wrapper.get('[data-test="user-avatar-image"]').trigger('error')
    expect(wrapper.find('[data-test="user-avatar-image"]').exists()).toBe(false)

    await wrapper.setProps({ avatarUrl: DATA_URL })

    const img = wrapper.get('[data-test="user-avatar-image"]')
    expect(img.attributes('src')).toBe(DATA_URL)
    expect(wrapper.find('[data-test="user-avatar-initial"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('applies the size preset classes', () => {
    const small = mount(UserAvatar, { props: { email: 'a@example.com', size: 'sm' } })
    expect(small.classes()).toEqual(expect.arrayContaining(['h-8', 'w-8']))
    expect(small.get('[data-test="user-avatar-initial"]').classes()).toContain('text-sm')
    small.unmount()

    const medium = mount(UserAvatar, { props: { email: 'a@example.com', size: 'md' } })
    expect(medium.classes()).toEqual(expect.arrayContaining(['h-10', 'w-10']))
    expect(medium.get('[data-test="user-avatar-initial"]').classes()).toContain('text-lg')
    medium.unmount()

    const large = mount(UserAvatar, { props: { email: 'a@example.com', size: 'lg' } })
    expect(large.classes()).toEqual(expect.arrayContaining(['h-14', 'w-14']))
    expect(large.get('[data-test="user-avatar-initial"]').classes()).toContain('text-2xl')
    large.unmount()
  })
})
