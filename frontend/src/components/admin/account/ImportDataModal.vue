<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.dataImportTitle')"
    width="wide"
    :close-on-click-outside="!showProfileEditor"
    @close="handleClose"
  >
    <form id="import-data-form" class="space-y-4" @submit.prevent="handleImport">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.dataImportHint') }}
      </div>
      <div
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-600 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-400"
      >
        {{ t('admin.accounts.dataImportWarning') }}
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.dataImportFile') }}</label>
        <div
          class="flex items-center justify-between gap-3 rounded-lg border border-dashed px-4 py-3 transition-colors"
          :class="dragActive
            ? 'border-primary-400 bg-primary-50/70 dark:border-primary-500 dark:bg-primary-900/20'
            : 'border-gray-300 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'"
          @dragenter.prevent="handleDragEnter"
          @dragover.prevent
          @dragleave.prevent="handleDragLeave"
          @drop.prevent="handleDrop"
        >
          <div class="min-w-0">
            <div class="truncate text-sm text-gray-700 dark:text-dark-200" :title="fileListTitle">
              {{ selectedFilesLabel || t('admin.accounts.dataImportSelectFile') }}
            </div>
            <div class="text-xs text-gray-500 dark:text-dark-400">
              JSON (.json)
              <span v-if="files.length > 1"> · {{ fileListTitle }}</span>
            </div>
          </div>
          <button type="button" class="btn btn-secondary shrink-0" @click="openFilePicker">
            {{ t('common.chooseFile') }}
          </button>
        </div>
        <input
          ref="fileInput"
          data-testid="account-import-file-input"
          type="file"
          class="hidden"
          accept="application/json,.json"
          multiple
          @change="handleFileChange"
        />
      </div>

      <section class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="flex flex-col gap-3 border-b border-gray-200 bg-gray-50/80 p-4 dark:border-dark-700 dark:bg-dark-800/50 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <div class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.accounts.dataImportProfilesTitle') }}
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.dataImportProfilesListHint') }}
            </p>
          </div>
          <div class="flex shrink-0 flex-wrap gap-2">
            <button
              type="button"
              class="btn btn-primary btn-sm"
              data-testid="new-import-profile"
              @click="openNewProfileEditor"
            >
              {{ t('admin.accounts.dataImportProfileNew') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" @click="profileFileInput?.click()">
              {{ t('admin.accounts.dataImportProfileImport') }}
            </button>
          </div>
          <input
            ref="profileFileInput"
            data-testid="profile-import-file-input"
            type="file"
            class="hidden"
            accept="application/json,.json"
            @change="handleProfileFileImport"
          />
        </div>

        <div v-if="profiles.length" class="divide-y divide-gray-200 dark:divide-dark-700" data-testid="import-profile-list">
          <article
            v-for="profile in profiles"
            :key="profile.id"
            :data-testid="`import-profile-row-${profile.id}`"
            class="p-4 transition-colors"
            :class="activeProfileId === profile.id
              ? 'bg-primary-50/60 dark:bg-primary-950/20'
              : 'bg-white dark:bg-dark-800'"
          >
            <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
              <div class="min-w-0 flex-1 space-y-3">
                <div class="flex flex-wrap items-center gap-2">
                  <h3 class="font-semibold text-gray-900 dark:text-white">{{ profile.name }}</h3>
                  <span
                    v-if="activeProfileId === profile.id"
                    class="rounded-full bg-primary-100 px-2 py-0.5 text-[11px] font-medium text-primary-700 dark:bg-primary-900/40 dark:text-primary-300"
                  >
                    {{ t('admin.accounts.dataImportProfileInUse') }}
                  </span>
                  <span class="text-xs text-gray-500 dark:text-gray-400">
                    {{ profileScopeLabel(profile) }}
                  </span>
                </div>

                <div>
                  <div class="mb-1 text-[11px] font-semibold uppercase tracking-wide text-gray-400">
                    {{ t('admin.accounts.dataImportProfileBatchSummary') }}
                  </div>
                  <div class="flex flex-wrap gap-1.5">
                    <span
                      v-for="item in profileBatchSummary(profile)"
                      :key="item"
                      class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-300"
                    >
                      {{ item }}
                    </span>
                  </div>
                </div>

                <div>
                  <div class="mb-1 text-[11px] font-semibold uppercase tracking-wide text-gray-400">
                    {{ t('admin.accounts.dataImportProfileProxySummary') }}
                  </div>
                  <div class="flex flex-wrap gap-1.5">
                    <span
                      v-for="item in profileProxySummary(profile)"
                      :key="item"
                      class="rounded-md px-2 py-1 text-xs"
                      :class="profile.smart_proxy_assignment.enabled
                        ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/25 dark:text-emerald-300'
                        : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'"
                    >
                      {{ item }}
                    </span>
                  </div>
                </div>

                <div class="text-[11px] text-gray-400">
                  {{ t('admin.accounts.dataImportProfileUpdatedAt', { time: formatProfileDate(profile.updated_at) }) }}
                </div>
              </div>

              <div class="flex shrink-0 flex-wrap gap-2 xl:max-w-[270px] xl:justify-end">
                <button
                  type="button"
                  class="btn btn-primary btn-sm"
                  :data-testid="`import-profile-${profile.id}`"
                  @click="applyProfile(profile)"
                >
                  {{ t('admin.accounts.dataImportProfileUse') }}
                </button>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :data-testid="`edit-import-profile-${profile.id}`"
                  @click="openEditProfileEditor(profile)"
                >
                  {{ t('common.edit') }}
                </button>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm"
                  :data-testid="`copy-import-profile-${profile.id}`"
                  @click="copyProfile(profile)"
                >
                  {{ t('common.copy') }}
                </button>
                <button type="button" class="btn btn-secondary btn-sm" @click="exportProfile(profile)">
                  {{ t('admin.accounts.dataImportProfileExport') }}
                </button>
                <button
                  type="button"
                  class="btn btn-danger btn-sm"
                  :data-testid="`delete-import-profile-${profile.id}`"
                  @click="deleteProfile(profile)"
                >
                  {{ t('common.delete') }}
                </button>
              </div>
            </div>
          </article>
        </div>
        <div v-else class="bg-white px-4 py-10 text-center dark:bg-dark-800" data-testid="import-profile-empty">
          <div class="text-sm font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.accounts.dataImportProfilesEmpty') }}
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.dataImportProfilesEmptyHint') }}
          </p>
          <button type="button" class="btn btn-primary btn-sm mt-4" @click="openNewProfileEditor">
            {{ t('admin.accounts.dataImportProfileNew') }}
          </button>
        </div>
      </section>

      <div
        v-if="result"
        class="space-y-2 rounded-xl border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.dataImportResult') }}
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.dataImportResultSummary', result) }}
        </div>

        <div v-if="errorItems.length" class="mt-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.accounts.dataImportErrors') }}
          </div>
          <div class="mt-2 max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800">
            <div v-for="(item, idx) in errorItems" :key="idx" class="whitespace-pre-wrap">
              {{ item.kind }} {{ item.name || item.proxy_key || '-' }} — {{ item.message }}
            </div>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="importing" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" type="submit" form="import-data-form" :disabled="importing">
          {{ importing ? t('admin.accounts.dataImporting') : t('admin.accounts.dataImportButton') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <BulkEditAccountModal
    :key="profileEditorKey"
    :show="showProfileEditor"
    :account-ids="[]"
    :selected-platforms="editorPlatforms"
    :selected-types="editorTypes"
    :target="{
      mode: 'selected',
      previewCount: importAccountCount,
      selectedPlatforms: editorPlatforms,
      selectedTypes: editorTypes
    }"
    :proxies="proxies"
    :groups="groups"
    :profile-name="editorProfileName"
    :initial-updates="editorInitialUpdates"
    :initial-smart-proxy-options="editorInitialSmartProxyOptions"
    config-only
    @close="showProfileEditor = false"
    @configured="handleProfileConfigured"
  />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import BulkEditAccountModal from '@/components/account/BulkEditAccountModal.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type {
  AccountPlatform,
  AccountType,
  AdminDataImportResult,
  AdminDataPayload,
  AdminGroup,
  Proxy,
  SmartProxyAssignmentOptions
} from '@/types'

interface Props {
  show: boolean
  proxies?: Proxy[]
  groups?: AdminGroup[]
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

interface ImportBatchProfile {
  id: string
  name: string
  post_import_updates: Record<string, unknown> | null
  smart_proxy_assignment: SmartProxyAssignmentOptions
  platforms: AccountPlatform[]
  account_types: AccountType[]
  updated_at: string
}

interface ImportBatchProfileFile {
  type: 'sub2api-account-import-profile'
  version: 1
  exported_at: string
  profile: ImportBatchProfile
}

interface ProfileEditorResult {
  name: string
  updates: Record<string, unknown> | null
  smart_proxy_assignment: SmartProxyAssignmentOptions
}

const props = withDefaults(defineProps<Props>(), {
  proxies: () => [],
  groups: () => []
})
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const importing = ref(false)
const IMPORT_PROFILE_STORAGE_KEY = 'sub2api:admin:account-import-profiles:v1'
const files = ref<File[]>([])
const dragDepth = ref(0)
const dragActive = computed(() => dragDepth.value > 0)
const hasCreatedData = ref(false)
const result = ref<AdminDataImportResult | null>(null)
const parsedPayloads = ref<AdminDataPayload[]>([])
const showProfileEditor = ref(false)
const profileEditorVersion = ref(0)
const editorProfileId = ref('')
const editorProfileName = ref('')
const editorInitialUpdates = ref<Record<string, unknown> | null>(null)
const editorInitialSmartProxyOptions = ref<SmartProxyAssignmentOptions>(defaultSmartProxyOptions())
const editorPlatforms = ref<AccountPlatform[]>([])
const editorTypes = ref<AccountType[]>([])
const postImportUpdates = ref<Record<string, unknown> | null>(null)
const smartProxyOptions = ref<SmartProxyAssignmentOptions>(defaultSmartProxyOptions())
const profiles = ref<ImportBatchProfile[]>([])
const activeProfileId = ref('')

const fileInput = ref<HTMLInputElement | null>(null)
const profileFileInput = ref<HTMLInputElement | null>(null)
const importedAccounts = computed(() => parsedPayloads.value.flatMap((payload) => payload.accounts))
const importAccountCount = computed(() => importedAccounts.value.length)
const importPlatforms = computed<AccountPlatform[]>(() => Array.from(new Set(
  importedAccounts.value.map((account) => account.platform).filter(Boolean)
)))
const importTypes = computed<AccountType[]>(() => Array.from(new Set(
  importedAccounts.value.map((account) => account.type).filter(Boolean)
)))
const profileEditorKey = computed(() => `${editorProfileId.value || 'new'}:${profileEditorVersion.value}`)
const selectedFilesLabel = computed(() => {
  if (files.value.length === 0) return ''
  if (files.value.length === 1) return files.value[0]?.name || ''
  return t('admin.accounts.selectedCount', { count: files.value.length })
})
const fileListTitle = computed(() => files.value.map((item) => item.name).join(', '))
const errorItems = computed(() => result.value?.errors || [])

function defaultSmartProxyOptions(): SmartProxyAssignmentOptions {
  return {
    enabled: false,
    proxy_count: 2,
    test_latency: true,
    prefer_low_latency: true,
    low_latency_limit: 0,
    weighted_by_load: true
  }
}

const cloneJSON = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T
const isRecord = (value: unknown): value is Record<string, unknown> =>
  Boolean(value) && typeof value === 'object' && !Array.isArray(value)

const makeProfileID = () => {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `profile-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

const normalizeSmartProxyOptions = (raw: unknown): SmartProxyAssignmentOptions => {
  const value = isRecord(raw) ? raw as Partial<SmartProxyAssignmentOptions> : {}
  return {
    enabled: value.enabled === true,
    proxy_count: Math.min(32, Math.max(1, Math.trunc(Number(value.proxy_count) || 2))),
    test_latency: value.test_latency !== false,
    prefer_low_latency: value.prefer_low_latency !== false,
    low_latency_limit: Math.max(0, Math.trunc(Number(value.low_latency_limit) || 0)),
    weighted_by_load: value.weighted_by_load !== false
  }
}

const normalizeImportProfile = (raw: unknown): ImportBatchProfile | null => {
  if (!isRecord(raw)) return null
  const value = raw as Partial<ImportBatchProfile>
  const name = typeof value.name === 'string' ? value.name.trim() : ''
  if (!name) return null
  const updates = isRecord(value.post_import_updates) ? cloneJSON(value.post_import_updates) : null
  return {
    id: typeof value.id === 'string' && value.id.trim() ? value.id.trim() : makeProfileID(),
    name,
    post_import_updates: updates,
    smart_proxy_assignment: normalizeSmartProxyOptions(value.smart_proxy_assignment),
    platforms: Array.isArray(value.platforms) ? Array.from(new Set(value.platforms)) : [],
    account_types: Array.isArray(value.account_types) ? Array.from(new Set(value.account_types)) : [],
    updated_at: typeof value.updated_at === 'string' ? value.updated_at : new Date().toISOString()
  }
}

const loadProfiles = () => {
  try {
    const raw = localStorage.getItem(IMPORT_PROFILE_STORAGE_KEY)
    const parsed = raw ? JSON.parse(raw) : []
    profiles.value = Array.isArray(parsed)
      ? parsed.map(normalizeImportProfile).filter((profile): profile is ImportBatchProfile => profile !== null)
      : []
  } catch {
    profiles.value = []
  }
}

const persistProfiles = () => {
  try {
    localStorage.setItem(IMPORT_PROFILE_STORAGE_KEY, JSON.stringify(profiles.value))
  } catch {
    appStore.showWarning(t('admin.accounts.dataImportProfileStorageFailed'))
  }
}

const profileScopeLabel = (profile: ImportBatchProfile) => {
  const platforms = profile.platforms.length ? profile.platforms.join(', ') : t('common.all')
  const types = profile.account_types.length ? profile.account_types.join(', ') : t('common.all')
  return `${platforms} / ${types}`
}

const formatProfileDate = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(date)
}

const profileBatchSummary = (profile: ImportBatchProfile): string[] => {
  const updates = profile.post_import_updates
  if (!updates) return [t('admin.accounts.dataImportProfileNoBatchConfig')]
  const credentials = isRecord(updates.credentials) ? updates.credentials : {}
  const extra = isRecord(updates.extra) ? updates.extra : {}
  const items: string[] = []
  const add = (key: string, value: unknown) => items.push(t(key, { value }))

  if ('concurrency' in updates) add('admin.accounts.dataImportProfileConcurrency', updates.concurrency)
  if ('load_factor' in updates) add('admin.accounts.dataImportProfileLoadFactor', updates.load_factor)
  if ('priority' in updates) add('admin.accounts.dataImportProfilePriority', updates.priority)
  if ('rate_multiplier' in updates) add('admin.accounts.dataImportProfileRateMultiplier', updates.rate_multiplier)
  if ('status' in updates) add('admin.accounts.dataImportProfileStatus', updates.status)
  if ('proxy_id' in updates) add('admin.accounts.dataImportProfileMainProxy', updates.proxy_id || t('common.none'))
  if (Array.isArray(updates.group_ids)) add('admin.accounts.dataImportProfileGroups', updates.group_ids.length)
  if ('upstream_billing_probe_enabled' in updates) {
    add('admin.accounts.dataImportProfileBillingProbe', updates.upstream_billing_probe_enabled ? t('common.enabled') : t('common.disabled'))
  }
  if ('base_url' in credentials) add('admin.accounts.dataImportProfileBaseUrl', credentials.base_url)
  if ('model_mapping' in credentials) {
    add('admin.accounts.dataImportProfileModels', isRecord(credentials.model_mapping) ? Object.keys(credentials.model_mapping).length : 0)
  }
  if (Array.isArray(credentials.custom_error_codes)) {
    add('admin.accounts.dataImportProfileErrorCodes', credentials.custom_error_codes.join(', '))
  }
  if ('intercept_warmup_requests' in credentials) {
    add('admin.accounts.dataImportProfileWarmup', credentials.intercept_warmup_requests ? t('common.enabled') : t('common.disabled'))
  }
  if ('header_overrides' in credentials) {
    add('admin.accounts.dataImportProfileHeaders', isRecord(credentials.header_overrides) ? Object.keys(credentials.header_overrides).length : 0)
  }
  if ('openai_capabilities' in credentials) add('admin.accounts.dataImportProfileCapabilities', Array.isArray(credentials.openai_capabilities) ? credentials.openai_capabilities.length : 2)
  if ('openai_passthrough' in extra) add('admin.accounts.dataImportProfilePassthrough', extra.openai_passthrough ? t('common.enabled') : t('common.disabled'))
  if ('openai_long_context_billing_enabled' in extra) add('admin.accounts.dataImportProfileLongContext', extra.openai_long_context_billing_enabled ? t('common.enabled') : t('common.disabled'))
  if ('openai_oauth_responses_websockets_v2_mode' in extra) add('admin.accounts.dataImportProfileOAuthWs', extra.openai_oauth_responses_websockets_v2_mode)
  if ('openai_apikey_responses_websockets_v2_mode' in extra) add('admin.accounts.dataImportProfileApiKeyWs', extra.openai_apikey_responses_websockets_v2_mode)
  if ('base_rpm' in extra) add('admin.accounts.dataImportProfileRpm', extra.base_rpm)
  if ('user_msg_queue_mode' in extra) add('admin.accounts.dataImportProfileUmq', extra.user_msg_queue_mode || t('common.disabled'))

  return items.length ? items : [t('admin.accounts.dataImportProfileBatchConfigured')]
}

const profileProxySummary = (profile: ImportBatchProfile): string[] => {
  const options = profile.smart_proxy_assignment
  if (!options.enabled) return [t('admin.accounts.dataImportProfileProxyDisabled')]
  return [
    t('admin.accounts.dataImportProfileProxyCount', { count: options.proxy_count }),
    options.test_latency
      ? t('admin.accounts.dataImportProfileLatencyTestEnabled')
      : t('admin.accounts.dataImportProfileLatencyTestDisabled'),
    options.low_latency_limit > 0
      ? t('admin.accounts.dataImportProfileFastestCandidates', { count: options.low_latency_limit })
      : t('admin.accounts.dataImportProfileAllHealthyCandidates'),
    options.prefer_low_latency
      ? t('admin.accounts.dataImportProfilePreferLatency')
      : t('admin.accounts.dataImportProfileUniformLatency'),
    options.weighted_by_load
      ? t('admin.accounts.dataImportProfileWeightedLoad')
      : t('admin.accounts.dataImportProfileUnweightedLoad')
  ]
}

const uniqueProfileName = (baseName: string, excludingID = '') => {
  const names = new Set(
    profiles.value.filter((profile) => profile.id !== excludingID).map((profile) => profile.name)
  )
  if (!names.has(baseName)) return baseName
  for (let i = 2; ; i += 1) {
    const candidate = `${baseName} (${i})`
    if (!names.has(candidate)) return candidate
  }
}

const openNewProfileEditor = () => {
  editorProfileId.value = ''
  editorProfileName.value = ''
  editorInitialUpdates.value = null
  editorInitialSmartProxyOptions.value = defaultSmartProxyOptions()
  editorPlatforms.value = [...importPlatforms.value]
  editorTypes.value = [...importTypes.value]
  profileEditorVersion.value += 1
  showProfileEditor.value = true
}

const openEditProfileEditor = (profile: ImportBatchProfile) => {
  editorProfileId.value = profile.id
  editorProfileName.value = profile.name
  editorInitialUpdates.value = profile.post_import_updates ? cloneJSON(profile.post_import_updates) : null
  editorInitialSmartProxyOptions.value = cloneJSON(profile.smart_proxy_assignment)
  editorPlatforms.value = profile.platforms.length ? [...profile.platforms] : [...importPlatforms.value]
  editorTypes.value = profile.account_types.length ? [...profile.account_types] : [...importTypes.value]
  profileEditorVersion.value += 1
  showProfileEditor.value = true
}

const setAppliedProfile = (profile: ImportBatchProfile) => {
  activeProfileId.value = profile.id
  postImportUpdates.value = profile.post_import_updates ? cloneJSON(profile.post_import_updates) : null
  smartProxyOptions.value = cloneJSON(profile.smart_proxy_assignment)
}

const applyProfile = (profile: ImportBatchProfile, notify = true) => {
  setAppliedProfile(profile)
  const currentPlatforms = new Set(importPlatforms.value)
  const currentTypes = new Set(importTypes.value)
  const scopeMismatch = files.value.length > 0 && (
    profile.platforms.some((platform) => !currentPlatforms.has(platform)) ||
    profile.account_types.some((accountType) => !currentTypes.has(accountType))
  )
  if (scopeMismatch) appStore.showWarning(t('admin.accounts.dataImportProfileScopeWarning'))
  if (notify) appStore.showSuccess(t('admin.accounts.dataImportProfileApplied', { name: profile.name }))
}

const handleProfileConfigured = (config: ProfileEditorResult) => {
  const name = config.name.trim()
  const duplicate = profiles.value.some((profile) =>
    profile.id !== editorProfileId.value && profile.name === name
  )
  if (duplicate) {
    appStore.showError(t('admin.accounts.dataImportProfileNameExists'))
    return
  }

  const now = new Date().toISOString()
  const index = profiles.value.findIndex((profile) => profile.id === editorProfileId.value)
  const profile: ImportBatchProfile = {
    id: index >= 0 ? profiles.value[index]!.id : makeProfileID(),
    name,
    post_import_updates: config.updates ? cloneJSON(config.updates) : null,
    smart_proxy_assignment: normalizeSmartProxyOptions(config.smart_proxy_assignment),
    platforms: [...editorPlatforms.value],
    account_types: [...editorTypes.value],
    updated_at: now
  }

  if (index >= 0) profiles.value[index] = profile
  else profiles.value.push(profile)
  persistProfiles()
  setAppliedProfile(profile)
  showProfileEditor.value = false
  appStore.showSuccess(t(index >= 0
    ? 'admin.accounts.dataImportProfileUpdated'
    : 'admin.accounts.dataImportProfileSaved', { name }))
}

const copyProfile = (source: ImportBatchProfile) => {
  const name = uniqueProfileName(t('admin.accounts.dataImportProfileCopyName', { name: source.name }))
  profiles.value.push({
    ...cloneJSON(source),
    id: makeProfileID(),
    name,
    updated_at: new Date().toISOString()
  })
  persistProfiles()
  appStore.showSuccess(t('admin.accounts.dataImportProfileCopied', { name }))
}

const deleteProfile = (profile: ImportBatchProfile) => {
  if (!confirm(t('admin.accounts.dataImportProfileDeleteConfirm', { name: profile.name }))) return
  profiles.value = profiles.value.filter((item) => item.id !== profile.id)
  if (activeProfileId.value === profile.id) {
    activeProfileId.value = ''
    postImportUpdates.value = null
    smartProxyOptions.value = defaultSmartProxyOptions()
  }
  persistProfiles()
}

const exportProfile = (profile: ImportBatchProfile) => {
  const payload: ImportBatchProfileFile = {
    type: 'sub2api-account-import-profile',
    version: 1,
    exported_at: new Date().toISOString(),
    profile
  }
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `sub2api-import-profile-${profile.name.replace(/[^\w\-.\u4e00-\u9fff]+/g, '-')}.json`
  anchor.click()
  URL.revokeObjectURL(url)
}

const handleProfileFileImport = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const sourceFile = input.files?.[0]
  input.value = ''
  if (!sourceFile) return
  try {
    const parsed = JSON.parse(await readFileAsText(sourceFile)) as Partial<ImportBatchProfileFile>
    if (parsed.type !== 'sub2api-account-import-profile' || parsed.version !== 1) {
      throw new Error('unsupported profile file')
    }
    const imported = normalizeImportProfile(parsed.profile)
    if (!imported) throw new Error('invalid profile')
    imported.id = makeProfileID()
    imported.name = uniqueProfileName(imported.name)
    imported.updated_at = new Date().toISOString()
    profiles.value.push(imported)
    persistProfiles()
    applyProfile(imported, false)
    appStore.showSuccess(t('admin.accounts.dataImportProfileImported', { name: imported.name }))
  } catch {
    appStore.showError(t('admin.accounts.dataImportProfileImportFailed'))
  }
}

watch(
  () => props.show,
  (open) => {
    if (!open) return
    files.value = []
    dragDepth.value = 0
    hasCreatedData.value = false
    result.value = null
    parsedPayloads.value = []
    showProfileEditor.value = false
    postImportUpdates.value = null
    smartProxyOptions.value = defaultSmartProxyOptions()
    activeProfileId.value = ''
    loadProfiles()
    if (fileInput.value) fileInput.value.value = ''
  },
  { immediate: true }
)

const openFilePicker = () => fileInput.value?.click()

const handleFileChange = async (event: Event) => {
  const target = event.target as HTMLInputElement
  await setSelectedFiles(target.files)
  target.value = ''
}

const handleClose = () => {
  if (importing.value) return
  if (hasCreatedData.value) {
    hasCreatedData.value = false
    emit('imported')
  }
  emit('close')
}

const isJsonFile = (sourceFile: File) => {
  const name = sourceFile.name.toLowerCase()
  return name.endsWith('.json') || sourceFile.type === 'application/json'
}

const setSelectedFiles = async (sourceFiles: FileList | File[] | null | undefined) => {
  if (importing.value) return
  const incoming = Array.from(sourceFiles || [])
  const picked = incoming.filter(isJsonFile)
  if (!picked.length) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }
  if (picked.length < incoming.length) {
    appStore.showWarning(t('admin.accounts.dataImportIgnoredFiles', { count: incoming.length - picked.length }))
  }
  const activeProfile = profiles.value.find((profile) => profile.id === activeProfileId.value)
  files.value = picked
  result.value = null
  parsedPayloads.value = []
  for (const sourceFile of picked) {
    try {
      const parsed = JSON.parse(await readFileAsText(sourceFile))
      if (isValidDataPayload(parsed)) parsedPayloads.value.push(parsed)
    } catch {
      // handleImport reports the existing localized per-file validation error.
    }
  }
  if (activeProfile) setAppliedProfile(activeProfile)
}

const handleDragEnter = () => {
  if (!importing.value) dragDepth.value += 1
}
const handleDragLeave = () => {
  dragDepth.value = Math.max(0, dragDepth.value - 1)
}
const handleDrop = async (event: DragEvent) => {
  dragDepth.value = 0
  if (!importing.value) await setSelectedFiles(event.dataTransfer?.files)
}

const readFileAsText = async (sourceFile: File): Promise<string> => {
  if (typeof sourceFile.text === 'function') return sourceFile.text()
  if (typeof sourceFile.arrayBuffer === 'function') {
    const buffer = await sourceFile.arrayBuffer()
    return new TextDecoder().decode(buffer)
  }
  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
    reader.readAsText(sourceFile)
  })
}

const SUPPORTED_DATA_TYPES = ['sub2api-data', 'sub2api-bundle']
const SUPPORTED_DATA_VERSION = 1

const isValidDataPayload = (payload: unknown): payload is AdminDataPayload => {
  if (!isRecord(payload)) return false
  if (payload.type !== undefined && payload.type !== '' && !SUPPORTED_DATA_TYPES.includes(payload.type as string)) return false
  if (payload.version !== undefined && payload.version !== 0 && payload.version !== SUPPORTED_DATA_VERSION) return false
  return Array.isArray(payload.proxies) && Array.isArray(payload.accounts)
}

const mergeDataPayloads = (payloads: AdminDataPayload[]): AdminDataPayload => {
  const [firstPayload] = payloads
  if (payloads.length === 1 && firstPayload) return firstPayload
  return {
    type: payloads.find((item) => typeof item.type === 'string')?.type,
    version: payloads.find((item) => typeof item.version === 'number')?.version,
    exported_at: new Date().toISOString(),
    proxies: payloads.flatMap((item) => item.proxies),
    accounts: payloads.flatMap((item) => item.accounts),
    skipped_shadows: payloads.reduce((sum, item) => {
      const count = Number(item.skipped_shadows || 0)
      return Number.isFinite(count) ? sum + count : sum
    }, 0)
  }
}

const handleImport = async () => {
  if (files.value.length === 0) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }

  importing.value = true
  try {
    const dataPayloads: AdminDataPayload[] = []
    for (const sourceFile of files.value) {
      let parsed: unknown
      try {
        parsed = JSON.parse(await readFileAsText(sourceFile))
      } catch {
        appStore.showError(t('admin.accounts.dataImportParseFailedFile', { name: sourceFile.name }))
        return
      }
      if (!isValidDataPayload(parsed)) {
        appStore.showError(t('admin.accounts.dataImportInvalidFile', { name: sourceFile.name }))
        return
      }
      dataPayloads.push(parsed)
    }

    const res = await adminAPI.accounts.importData({
      data: mergeDataPayloads(dataPayloads),
      skip_default_group_bind: true,
      post_import_updates: postImportUpdates.value ?? undefined,
      smart_proxy_assignment: smartProxyOptions.value.enabled ? smartProxyOptions.value : undefined
    })
    result.value = res

    const msgParams: Record<string, unknown> = {
      account_created: res.account_created,
      account_failed: res.account_failed,
      proxy_created: res.proxy_created,
      proxy_reused: res.proxy_reused,
      proxy_failed: res.proxy_failed,
      proxy_assigned: res.proxy_assigned || 0,
      proxy_assign_failed: res.proxy_assign_failed || 0,
      post_import_updated: res.post_import_updated || 0,
      post_import_failed: res.post_import_failed || 0
    }
    if (
      res.account_failed > 0 ||
      res.proxy_failed > 0 ||
      (res.proxy_assign_failed || 0) > 0 ||
      (res.post_import_failed || 0) > 0
    ) {
      if (res.account_created > 0 || res.proxy_created > 0) hasCreatedData.value = true
      appStore.showError(t('admin.accounts.dataImportCompletedWithErrors', msgParams))
    } else {
      appStore.showSuccess(t('admin.accounts.dataImportSuccess', msgParams))
      emit('imported')
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.dataImportFailed'))
  } finally {
    importing.value = false
  }
}
</script>
