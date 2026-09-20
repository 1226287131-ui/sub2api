import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import ProxyPoolSelector from '../ProxyPoolSelector.vue'

const proxy = (id: number, status: 'active' | 'inactive' | 'expired' = 'active') => ({
  id,
  name: `proxy-${id}`,
  protocol: 'http' as const,
  host: '127.0.0.1',
  port: 8000 + id,
  username: null,
  status,
  expires_at: null,
  fallback_mode: 'none' as const,
  expiry_warn_days: 0,
  created_at: '',
  updated_at: ''
})

describe('ProxyPoolSelector', () => {
  it('excludes the primary and unavailable proxies, then emits selected pool ids', async () => {
    const wrapper = mount(ProxyPoolSelector, {
      props: {
        modelValue: [],
        primaryProxyId: 1,
        proxies: [proxy(1), proxy(2), proxy(3, 'inactive')]
      },
      global: { stubs: { Icon: true } }
    })

    await wrapper.get('button.select-trigger').trigger('click')
    const options = wrapper.find('div.max-h-64')
    expect(options.text()).not.toContain('proxy-1')
    expect(options.text()).toContain('proxy-2')
    expect(options.text()).not.toContain('proxy-3')
    await options.get('input[type="checkbox"]').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([2])
  })

  it('does not erase persisted ids while the proxy list is still loading', () => {
    const wrapper = mount(ProxyPoolSelector, {
      props: { modelValue: [9], proxies: [] },
      global: { stubs: { Icon: true } }
    })
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('keeps legacy limits untouched until a lane is explicitly edited', async () => {
    const wrapper = mount(ProxyPoolSelector, {
      props: { modelValue: [], primaryProxyId: 1, proxies: [], accountConcurrency: 0 },
      global: { stubs: { Icon: true } }
    })
    await wrapper.setProps({ proxies: [proxy(1), proxy(2)] })
    await wrapper.setProps({ accountConcurrency: 7 })
    expect(wrapper.emitted('update:laneConfigs')).toBeUndefined()

    await wrapper.get('[data-testid="proxy-lane-config-1"] input[type="number"]').setValue(3)
    expect(wrapper.emitted('update:laneConfigs')?.at(-1)?.[0]).toEqual([
      expect.objectContaining({ proxy_id: 1, max_concurrency: 3, enabled: true })
    ])
  })

  it('initializes independent limits when a proxy pool is selected', async () => {
    const wrapper = mount(ProxyPoolSelector, {
      props: { modelValue: [], primaryProxyId: 1, proxies: [proxy(1), proxy(2)], accountConcurrency: 6 },
      global: { stubs: { Icon: true } }
    })
    expect(wrapper.emitted('update:laneConfigs')).toBeUndefined()

    await wrapper.setProps({ modelValue: [2] })
    expect(wrapper.emitted('update:laneConfigs')?.at(-1)?.[0]).toEqual([
      expect.objectContaining({ proxy_id: 1, max_concurrency: 3, enabled: true }),
      expect.objectContaining({ proxy_id: 2, max_concurrency: 3, enabled: true })
    ])
  })
})
