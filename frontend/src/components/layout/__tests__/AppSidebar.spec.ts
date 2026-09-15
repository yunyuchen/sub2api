import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar collapsible groups', () => {
  it('lets the user collapse a group even while a child route is active', () => {
    // The expand state must come from the user's override first, falling back
    // to the active-route heuristic only when the user has not clicked yet.
    expect(componentSource).toContain('const groupExpandOverrides = ref<Map<string, boolean>>(new Map())')
    expect(componentSource).not.toContain('expandedGroups.value.has(item.path) || isGroupActive(item)')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

// ---------------------------------------------------------------------------
// Leaderboard 入口：随 isLeaderboardVisible() 显隐（off 档下对所有角色隐藏）。
// 这里真正挂载组件跑一遍 featureFlag 过滤，而不是只比对源码文本。
// ---------------------------------------------------------------------------
import { mount, RouterLinkStub } from '@vue/test-utils'
import { beforeEach, vi } from 'vitest'
import { ref } from 'vue'

const leaderboardVisible = vi.hoisted(() => vi.fn(() => false))

const sidebarStores = vi.hoisted(() => ({
  app: {
    sidebarCollapsed: false,
    mobileOpen: false,
    siteName: 'Sub2API',
    siteLogo: '',
    siteVersion: '0.0.0',
    publicSettingsLoaded: true,
    backendModeEnabled: false,
    cachedPublicSettings: { custom_menu_items: [] } as Record<string, unknown>,
    sidebarScrollTop: 0,
    toggleSidebar: vi.fn(),
    setMobileOpen: vi.fn(),
  },
  auth: {
    isAdmin: false,
    isSimpleMode: false,
  },
  adminSettings: {
    opsMonitoringEnabled: false,
    paymentEnabled: false,
    customMenuItems: [] as unknown[],
    fetch: vi.fn(),
  },
  onboarding: {
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => sidebarStores.app,
  useAuthStore: () => sidebarStores.auth,
  useAdminSettingsStore: () => sidebarStores.adminSettings,
  useOnboardingStore: () => sidebarStores.onboarding,
}))

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({
    canUseBatchImage: ref(false),
    refreshBatchImageAccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, te: () => true }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/dashboard' }),
  useRouter: () => ({ push: vi.fn() }),
}))

vi.mock('@/utils/featureFlags', () => ({
  FeatureFlags: {
    channelMonitor: { key: 'channel_monitor_enabled', mode: 'opt-out', label: 'Channel Monitor' },
    availableChannels: { key: 'available_channels_enabled', mode: 'opt-in', label: 'Available Channels' },
    modelPlaza: { key: 'model_plaza_enabled', mode: 'opt-in', label: 'Model Plaza' },
    pluginManagement: { key: 'plugin_management_enabled', mode: 'opt-in', label: 'Plugin Management' },
    payment: { key: 'payment_enabled', mode: 'opt-out', label: 'Payment' },
    riskControl: { key: 'risk_control_enabled', mode: 'opt-in', label: 'Risk Control' },
    affiliate: { key: 'affiliate_enabled', mode: 'opt-in', label: 'Affiliate' },
  },
  makeSidebarFlag: () => () => false,
  isLeaderboardVisible: () => leaderboardVisible(),
}))

async function mountSidebar() {
  const { default: AppSidebar } = await import('../AppSidebar.vue')
  return mount(AppSidebar, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        VersionBadge: true,
        Icon: true,
      },
    },
  })
}

function sidebarPaths(wrapper: Awaited<ReturnType<typeof mountSidebar>>): string[] {
  return wrapper
    .findAllComponents(RouterLinkStub)
    .map((link) => String(link.props('to')))
}

describe('AppSidebar leaderboard entry', () => {
  beforeEach(() => {
    leaderboardVisible.mockReset()
    sidebarStores.auth.isAdmin = false
    sidebarStores.auth.isSimpleMode = false
  })

  it('hides the entry when the leaderboard mode is off', async () => {
    leaderboardVisible.mockReturnValue(false)
    const wrapper = await mountSidebar()
    expect(sidebarPaths(wrapper)).not.toContain('/leaderboard')
  })

  it('shows the entry when the leaderboard mode is not off', async () => {
    leaderboardVisible.mockReturnValue(true)
    const wrapper = await mountSidebar()
    expect(sidebarPaths(wrapper)).toContain('/leaderboard')
  })

  it('hides the entry from admins too while the mode is off', async () => {
    // off 档下入口对所有角色隐藏；管理员经直链 /leaderboard 进入 Preview。
    leaderboardVisible.mockReturnValue(false)
    sidebarStores.auth.isAdmin = true
    const wrapper = await mountSidebar()
    expect(sidebarPaths(wrapper)).not.toContain('/leaderboard')
  })

  it('shows the entry to admins in their personal section once enabled', async () => {
    leaderboardVisible.mockReturnValue(true)
    sidebarStores.auth.isAdmin = true
    const wrapper = await mountSidebar()
    expect(sidebarPaths(wrapper)).toContain('/leaderboard')
  })

  it('hides the entry in simple mode, matching /usage', async () => {
    leaderboardVisible.mockReturnValue(true)
    sidebarStores.auth.isSimpleMode = true
    const wrapper = await mountSidebar()
    const paths = sidebarPaths(wrapper)
    expect(paths).not.toContain('/usage')
    expect(paths).not.toContain('/leaderboard')
  })
})

describe('AppSidebar subscription feature flag', () => {
  it('gates the My Subscriptions entry behind the subscription public-settings flag', () => {
    expect(componentSource).toContain('const flagSubscription = makeSidebarFlag(FeatureFlags.subscription)')
    expect(componentSource).toMatch(/path: '\/subscriptions'[^\n]*featureFlag: flagSubscription/)
  })

  it('also hides the admin Subscription Management entry on recharge-only sites', () => {
    expect(componentSource).toMatch(/path: '\/admin\/subscriptions'[^\n]*featureFlag: flagSubscription/)
  })

  it('derives the purchase entry label from the site billing mode', () => {
    expect(componentSource).toContain("import { resolveSiteBillingMode } from '@/utils/siteBillingMode'")
    expect(componentSource).toMatch(/case 'recharge_only':\s*return t\('nav\.recharge'\)/)
    expect(componentSource).toMatch(/case 'subscription_only':\s*return t\('nav\.subscribe'\)/)
    expect(componentSource).toMatch(/path: '\/purchase'[^\n]*label: purchaseNavLabel\.value/)
  })
})
