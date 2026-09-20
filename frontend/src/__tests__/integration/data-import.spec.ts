import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'

const showError = vi.fn()
const showSuccess = vi.fn()
const showWarning = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      importData: vi.fn()
    }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const mountModal = () =>
  mount(ImportDataModal, {
    props: { show: true },
    global: {
      stubs: {
        BaseDialog: {
          props: { show: Boolean },
          template: '<div v-if="show"><slot /><slot name="footer" /></div>'
        },
        GroupSelector: true,
        ProxySelector: true,
        ModelWhitelistSelector: true,
        ConfirmDialog: true,
        Icon: true
      }
    }
  })

const makeJsonFile = (name: string, content: string, type = 'application/json') => {
  const file = new File([content], name, { type })
  Object.defineProperty(file, 'text', {
    value: () => Promise.resolve(content)
  })
  return file
}

const setInputFiles = (element: Element, files: File[]) => {
  Object.defineProperty(element, 'files', {
    value: files,
    configurable: true
  })
}

describe('ImportDataModal', () => {
  beforeEach(async () => {
    localStorage.clear()
    vi.stubGlobal('confirm', vi.fn(() => true))
    showError.mockReset()
    showSuccess.mockReset()
    showWarning.mockReset()
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockReset()
  })

  it('未选择文件时提示错误', async () => {
    const wrapper = mountModal()

    await wrapper.find('form').trigger('submit')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')
  })

  it('无效 JSON 时按文件名提示解析失败', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()

    const input = wrapper.find('[data-testid="account-import-file-input"]')
    setInputFiles(input.element, [makeJsonFile('data.json', 'invalid json')])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailedFile')
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('不是导出数据的 JSON 按文件名拒绝', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()

    const input = wrapper.find('[data-testid="account-import-file-input"]')
    setInputFiles(input.element, [makeJsonFile('random.json', JSON.stringify({ name: 'test' }))])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportInvalidFile')
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('无有效 JSON 的选择不清空已有选择', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0
    })

    const wrapper = mountModal()
    const input = wrapper.find('[data-testid="account-import-file-input"]')

    const valid = makeJsonFile(
      'valid.json',
      JSON.stringify({ exported_at: '2026-07-05T00:00:00Z', proxies: [], accounts: [{ name: 'a' }] })
    )
    setInputFiles(input.element, [valid])
    await input.trigger('change')

    setInputFiles(input.element, [new File(['hello'], 'notes.txt', { type: 'text/plain' })])
    await input.trigger('change')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [{ name: 'a' }]
      }),
      skip_default_group_bind: true
    })
  })

  it('merges multiple selected JSON files before importing', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0
    })

    const wrapper = mountModal()

    const input = wrapper.find('[data-testid="account-import-file-input"]')
    const first = makeJsonFile(
      'first.json',
      JSON.stringify({ exported_at: '2026-07-05T00:00:00Z', proxies: [], accounts: [{ name: 'a' }] })
    )
    const second = makeJsonFile(
      'second.json',
      JSON.stringify({
        exported_at: '2026-07-05T00:00:01Z',
        proxies: [{ proxy_key: 'p' }],
        accounts: [{ name: 'b' }]
      })
    )
    setInputFiles(input.element, [first, second])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        proxies: [{ proxy_key: 'p' }],
        accounts: [{ name: 'a' }, { name: 'b' }]
      }),
      skip_default_group_bind: true
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.dataImportSuccess')
  })

  it('配置方案可同时保存批量编辑和智能代理并用于导入', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 1,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0,
      proxy_assigned: 1,
      proxy_assign_failed: 0
    })
    const wrapper = mountModal()
    const input = wrapper.find('[data-testid="account-import-file-input"]')
    setInputFiles(input.element, [makeJsonFile(
      'data.json',
      JSON.stringify({
        exported_at: '2026-07-05T00:00:00Z',
        proxies: [],
        accounts: [{ name: 'a', platform: 'openai', type: 'apikey' }]
      })
    )])
    await input.trigger('change')
    await wrapper.get('[data-testid="new-import-profile"]').trigger('click')
    await wrapper.get('[data-testid="import-profile-editor-name"]').setValue('代理并发')
    await wrapper.get('#bulk-edit-concurrency-enabled').setValue(true)
    await wrapper.get('#bulk-edit-concurrency').setValue(6)
    await wrapper.get('[data-testid="smart-proxy-enabled"]').setValue(true)
    await wrapper.get('[data-testid="smart-proxy-count"]').setValue(3)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await wrapper.get('#import-data-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith(expect.objectContaining({
      post_import_updates: { concurrency: 6 },
      smart_proxy_assignment: {
        enabled: true,
        proxy_count: 3,
        test_latency: true,
        prefer_low_latency: true,
        low_latency_limit: 0,
        weighted_by_load: true
      }
    }))
  })

  it('以列表保存多套方案并一键切换', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0,
      post_import_updated: 1,
      post_import_failed: 0
    })
    const wrapper = mountModal()
    const input = wrapper.find('[data-testid="account-import-file-input"]')
    setInputFiles(input.element, [makeJsonFile(
      'data.json',
      JSON.stringify({
        exported_at: '2026-07-05T00:00:00Z',
        proxies: [],
        accounts: [{ name: 'a', platform: 'openai', type: 'apikey' }]
      })
    )])
    await input.trigger('change')
    await flushPromises()

    const configure = async (concurrency: number, name: string) => {
      await wrapper.get('[data-testid="new-import-profile"]').trigger('click')
      await wrapper.get('[data-testid="import-profile-editor-name"]').setValue(name)
      await wrapper.get('#bulk-edit-concurrency-enabled').setValue(true)
      await wrapper.get('#bulk-edit-concurrency').setValue(concurrency)
      await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
      await flushPromises()
    }

    await configure(4, '低并发')
    await configure(12, '高并发')
    expect(wrapper.findAll('[data-testid^="import-profile-row-"]')).toHaveLength(2)

    const lowProfile = wrapper.findAll('[data-testid^="import-profile-row-"]').find(
      (row) => row.text().includes('低并发')
    )
    expect(lowProfile).toBeDefined()
    await lowProfile!.find('button.btn-primary').trigger('click')
    await wrapper.get('#import-data-form').trigger('submit.prevent')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith(expect.objectContaining({
      post_import_updates: { concurrency: 4 }
    }))
    const stored = JSON.parse(localStorage.getItem('sub2api:admin:account-import-profiles:v1') || '[]')
    expect(stored.map((profile: { name: string }) => profile.name)).toEqual(['低并发', '高并发'])
  })

  it('读取已保存方案并在编辑器中完整反填', async () => {
    localStorage.setItem('sub2api:admin:account-import-profiles:v1', JSON.stringify([{
      id: 'saved-profile',
      name: '常用方案',
      post_import_updates: { priority: 8 },
      smart_proxy_assignment: {
        enabled: false,
        proxy_count: 2,
        test_latency: true,
        prefer_low_latency: true,
        low_latency_limit: 0,
        weighted_by_load: true
      },
      platforms: ['openai'],
      account_types: ['apikey'],
      updated_at: '2026-09-19T00:00:00Z'
    }]))
    const wrapper = mountModal()
    expect(wrapper.text()).toContain('常用方案')
    expect(wrapper.get('[data-testid="import-profile-row-saved-profile"]').text()).toContain(
      'admin.accounts.dataImportProfilePriority'
    )
    await wrapper.get('[data-testid="edit-import-profile-saved-profile"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('[data-testid="import-profile-editor-name"]').element.value).toBe('常用方案')
    expect(wrapper.get<HTMLInputElement>('#bulk-edit-priority-enabled').element.checked).toBe(true)
    expect(wrapper.get<HTMLInputElement>('#bulk-edit-priority').element.value).toBe('8')
  })

  it('支持复制、删除和导入配置方案', async () => {
    localStorage.setItem('sub2api:admin:account-import-profiles:v1', JSON.stringify([{
      id: 'base-profile',
      name: '基础方案',
      post_import_updates: { concurrency: 4 },
      smart_proxy_assignment: {
        enabled: true,
        proxy_count: 2,
        test_latency: true,
        prefer_low_latency: true,
        low_latency_limit: 10,
        weighted_by_load: true
      },
      platforms: ['openai'],
      account_types: ['apikey'],
      updated_at: '2026-09-19T00:00:00Z'
    }]))
    const wrapper = mountModal()

    await wrapper.get('[data-testid="copy-import-profile-base-profile"]').trigger('click')
    expect(wrapper.findAll('[data-testid^="import-profile-row-"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('admin.accounts.dataImportProfileCopyName')

    await wrapper.get('[data-testid="delete-import-profile-base-profile"]').trigger('click')
    expect(wrapper.findAll('[data-testid^="import-profile-row-"]')).toHaveLength(1)

    const importedProfile = {
      type: 'sub2api-account-import-profile',
      version: 1,
      exported_at: '2026-09-19T01:00:00Z',
      profile: {
        id: 'external',
        name: '外部方案',
        post_import_updates: { priority: 9 },
        smart_proxy_assignment: {
          enabled: false,
          proxy_count: 2,
          test_latency: true,
          prefer_low_latency: true,
          low_latency_limit: 0,
          weighted_by_load: true
        },
        platforms: ['openai'],
        account_types: ['oauth'],
        updated_at: '2026-09-19T01:00:00Z'
      }
    }
    const profileInput = wrapper.get('[data-testid="profile-import-file-input"]')
    setInputFiles(profileInput.element, [makeJsonFile('profile.json', JSON.stringify(importedProfile))])
    await profileInput.trigger('change')
    await flushPromises()
    expect(wrapper.text()).toContain('外部方案')
    expect(wrapper.findAll('[data-testid^="import-profile-row-"]')).toHaveLength(2)
  })

  it('部分成功时关闭弹窗仍通知父组件刷新', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 1
    })

    const wrapper = mountModal()
    const input = wrapper.find('[data-testid="account-import-file-input"]')
    setInputFiles(input.element, [
      makeJsonFile(
        'mixed.json',
        JSON.stringify({
          exported_at: '2026-07-05T00:00:00Z',
          proxies: [],
          accounts: [{ name: 'a' }, { name: 'b' }]
        })
      )
    ])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportCompletedWithErrors')
    expect(wrapper.emitted('imported')).toBeUndefined()

    const cancelButton = wrapper.findAll('button.btn-secondary').find(
      (button) => button.text() === 'common.cancel'
    )
    expect(cancelButton).toBeDefined()
    await cancelButton!.trigger('click')

    expect(wrapper.emitted('imported')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
