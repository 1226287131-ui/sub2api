import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ProxyLanesCell from '../ProxyLanesCell.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const account = {
  id: 7,
  name: 'lane-account',
  platform: 'openai',
  type: 'oauth',
  proxy_id: 1,
  proxy_pool_ids: [2],
  proxy_lane_strategy: 'least_connections',
  proxy_lanes: [
    {
      proxy_id: 1,
      name: '主代理',
      primary: true,
      enabled: true,
      healthy: true,
      current_concurrency: 2,
      max_concurrency: 5,
      weight: 1,
      timeout_seconds: 30,
      error_circuit_threshold: 3,
      circuit_cooldown_seconds: 60,
      fallback_order: 0,
      status: 'active',
      expires_at: null
    },
    {
      proxy_id: 2,
      name: '代理 A',
      primary: false,
      enabled: true,
      healthy: true,
      current_concurrency: 1,
      max_concurrency: 5,
      weight: 2,
      timeout_seconds: 0,
      error_circuit_threshold: 0,
      circuit_cooldown_seconds: 60,
      fallback_order: 1,
      status: 'active',
      expires_at: null
    }
  ]
} as any

const proxies = [
  { id: 1, latency_ms: 88 },
  { id: 2, latency_ms: 120 }
] as any

describe('ProxyLanesCell', () => {
  it('shows every lane with independent n/m, latency, and aggregate strategy', () => {
    const wrapper = mount(ProxyLanesCell, { props: { account, proxies } })

    expect(wrapper.text()).toContain('主代理')
    expect(wrapper.text()).toContain('代理 A')
    expect(wrapper.text()).toContain('2/5')
    expect(wrapper.text()).toContain('1/5')
    expect(wrapper.text()).toContain('88ms')
    expect(wrapper.text()).toContain('120ms')
    expect(wrapper.text()).toContain('3/10')
    expect(wrapper.text()).toContain('admin.accounts.proxyLanes.strategy.least_connections')
  })
})
