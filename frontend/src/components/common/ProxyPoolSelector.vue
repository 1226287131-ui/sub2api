<template>
  <div class="space-y-2">
    <button
      type="button"
      class="select-trigger"
      :class="disabled && 'select-trigger-disabled'"
      :disabled="disabled"
      @click="isOpen = !isOpen"
    >
      <span class="select-value">
        {{ selectedLabel }}
      </span>
      <Icon
        name="chevronDown"
        size="md"
        :class="['transition-transform duration-200', isOpen && 'rotate-180']"
      />
    </button>

    <div
      v-if="isOpen"
      class="max-h-64 overflow-y-auto rounded-lg border border-gray-200 bg-white p-2 shadow-sm dark:border-dark-500 dark:bg-dark-700"
    >
      <div class="mb-2 flex items-center gap-2 rounded-md bg-gray-50 px-2 dark:bg-dark-600">
        <Icon name="search" size="sm" class="text-gray-400" />
        <input
          v-model="searchQuery"
          type="text"
          class="min-w-0 flex-1 bg-transparent py-2 text-sm outline-none dark:text-white"
          :placeholder="t('admin.proxies.searchProxies')"
        />
      </div>

      <label
        v-for="proxy in filteredProxies"
        :key="proxy.id"
        class="flex cursor-pointer items-center gap-3 rounded-md px-2 py-2 hover:bg-gray-50 dark:hover:bg-dark-600"
      >
        <input
          type="checkbox"
          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          :checked="modelValue.includes(proxy.id)"
          @change="toggleProxy(proxy.id)"
        />
        <span class="min-w-0 flex-1">
          <span class="block truncate text-sm font-medium text-gray-800 dark:text-gray-100">
            {{ proxy.name }}
          </span>
          <span class="block truncate text-xs text-gray-500 dark:text-gray-400">
            {{ proxy.protocol }}://{{ proxy.host }}:{{ proxy.port }}
          </span>
        </span>
      </label>

      <p v-if="filteredProxies.length === 0" class="px-2 py-3 text-center text-sm text-gray-500">
        {{ t('common.noOptionsFound') }}
      </p>
    </div>

    <div v-if="selectedProxies.length > 0" class="flex flex-wrap gap-1.5">
      <button
        v-for="proxy in selectedProxies"
        :key="proxy.id"
        type="button"
        class="inline-flex items-center gap-1 rounded-full bg-primary-50 px-2 py-1 text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
        :title="t('common.remove')"
        @click="toggleProxy(proxy.id)"
      >
        {{ proxy.name }}
        <span aria-hidden="true">×</span>
      </button>
    </div>

    <div v-if="laneProxies.length > 0" class="mt-3 space-y-3 rounded-xl border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800/60">
      <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.accounts.proxyLanes.title') }}
          </div>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.proxyLanes.hint') }}
          </p>
        </div>
        <select
          :value="laneStrategy"
          class="input min-w-44 py-1.5 text-xs"
          data-testid="proxy-lane-strategy"
          @change="updateLaneStrategy(($event.target as HTMLSelectElement).value as ProxyLaneStrategy)"
        >
          <option value="round_robin">{{ t('admin.accounts.proxyLanes.roundRobin') }}</option>
          <option value="least_connections">{{ t('admin.accounts.proxyLanes.leastConnections') }}</option>
          <option value="weighted">{{ t('admin.accounts.proxyLanes.weighted') }}</option>
        </select>
      </div>

      <div class="space-y-2">
        <div
          v-for="(proxy, index) in laneProxies"
          :key="proxy.id"
          class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-700"
          :data-testid="`proxy-lane-config-${proxy.id}`"
        >
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="min-w-0">
              <span class="font-medium text-gray-900 dark:text-white">{{ proxy.name }}</span>
              <span
                v-if="proxy.id === primaryProxyId"
                class="ml-2 rounded-full bg-blue-50 px-2 py-0.5 text-[11px] text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
              >
                {{ t('admin.accounts.proxyLanes.primary') }}
              </span>
              <span class="ml-2 text-xs text-gray-400">#{{ index + 1 }}</span>
            </div>
            <label class="inline-flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
              <input
                :checked="laneConfig(proxy.id).enabled"
                type="checkbox"
                class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                @change="updateLane(proxy.id, 'enabled', ($event.target as HTMLInputElement).checked)"
              />
              {{ t('common.enabled') }}
            </label>
          </div>

          <div class="mt-3 grid grid-cols-2 gap-2 lg:grid-cols-4">
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.proxyLanes.maxConcurrency') }}
              <input
                :value="laneConfig(proxy.id).max_concurrency"
                type="number"
                min="1"
                max="10000"
                class="input mt-1 py-1.5 text-xs"
                @input="updateLaneNumber(proxy.id, 'max_concurrency', $event, 1)"
              />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.proxyLanes.weight') }}
              <input
                :value="laneConfig(proxy.id).weight"
                type="number"
                min="1"
                max="1000"
                class="input mt-1 py-1.5 text-xs"
                @input="updateLaneNumber(proxy.id, 'weight', $event, 1)"
              />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.proxyLanes.timeout') }}
              <input
                :value="laneConfig(proxy.id).timeout_seconds"
                type="number"
                min="0"
                max="86400"
                class="input mt-1 py-1.5 text-xs"
                @input="updateLaneNumber(proxy.id, 'timeout_seconds', $event, 0)"
              />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.proxyLanes.fallbackOrder') }}
              <input
                :value="laneConfig(proxy.id).fallback_order"
                type="number"
                min="0"
                max="1000"
                class="input mt-1 py-1.5 text-xs"
                @input="updateLaneNumber(proxy.id, 'fallback_order', $event, 0)"
              />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.proxyLanes.circuitThreshold') }}
              <input
                :value="laneConfig(proxy.id).error_circuit_threshold"
                type="number"
                min="0"
                max="1000"
                class="input mt-1 py-1.5 text-xs"
                @input="updateLaneNumber(proxy.id, 'error_circuit_threshold', $event, 0)"
              />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.proxyLanes.circuitCooldown') }}
              <input
                :value="laneConfig(proxy.id).circuit_cooldown_seconds"
                type="number"
                min="0"
                max="86400"
                class="input mt-1 py-1.5 text-xs"
                @input="updateLaneNumber(proxy.id, 'circuit_cooldown_seconds', $event, 0)"
              />
            </label>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { Proxy, ProxyLaneConfig, ProxyLaneStrategy } from '@/types'

const props = withDefaults(defineProps<{
  modelValue: number[]
  proxies: Proxy[]
  primaryProxyId?: number | null
  laneConfigs?: ProxyLaneConfig[]
  laneStrategy?: ProxyLaneStrategy
  accountConcurrency?: number
  disabled?: boolean
}>(), {
  primaryProxyId: null,
  laneConfigs: () => [],
  laneStrategy: 'round_robin',
  accountConcurrency: 1,
  disabled: false
})

const emit = defineEmits<{
  'update:modelValue': [value: number[]]
  'update:laneConfigs': [value: ProxyLaneConfig[]]
  'update:laneStrategy': [value: ProxyLaneStrategy]
}>()

const { t } = useI18n()
const isOpen = ref(false)
const searchQuery = ref('')

const isUsable = (proxy: Proxy) => {
  if (proxy.status !== 'active') return false
  if (!proxy.expires_at) return true
  const expiresAt = Date.parse(proxy.expires_at)
  return Number.isNaN(expiresAt) || expiresAt > Date.now()
}

const selectableProxies = computed(() => props.proxies.filter((proxy) =>
  proxy.id !== props.primaryProxyId && isUsable(proxy)
))

const filteredProxies = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return selectableProxies.value
  return selectableProxies.value.filter((proxy) =>
    proxy.name.toLowerCase().includes(query) || proxy.host.toLowerCase().includes(query)
  )
})

const selectedProxies = computed(() => {
  const selected = new Set(props.modelValue)
  return selectableProxies.value.filter((proxy) => selected.has(proxy.id))
})

const laneProxies = computed(() => {
  const selected = new Set(props.modelValue)
  return props.proxies.filter((proxy) =>
    isUsable(proxy) && (proxy.id === props.primaryProxyId || selected.has(proxy.id))
  )
})

const defaultLaneConcurrency = computed(() => Math.max(
  1,
  Math.ceil(Math.max(1, props.accountConcurrency) / Math.max(1, laneProxies.value.length))
))

const defaultLane = (proxyID: number, index = 0): ProxyLaneConfig => ({
  proxy_id: proxyID,
  enabled: true,
  max_concurrency: defaultLaneConcurrency.value,
  weight: 1,
  timeout_seconds: 0,
  error_circuit_threshold: 0,
  circuit_cooldown_seconds: 60,
  fallback_order: index
})

const laneConfig = (proxyID: number): ProxyLaneConfig => {
  const found = props.laneConfigs.find((item) => item.proxy_id === proxyID)
  return found || defaultLane(proxyID, laneProxies.value.findIndex((proxy) => proxy.id === proxyID))
}

const normalizedLaneConfigs = (): ProxyLaneConfig[] => laneProxies.value.map((proxy, index) => ({
  ...defaultLane(proxy.id, index),
  ...laneConfig(proxy.id),
  proxy_id: proxy.id
}))

const updateLane = <K extends keyof ProxyLaneConfig>(
  proxyID: number,
  key: K,
  value: ProxyLaneConfig[K]
) => {
  emit('update:laneConfigs', normalizedLaneConfigs().map((lane) =>
    lane.proxy_id === proxyID ? { ...lane, [key]: value } : lane
  ))
}

const updateLaneNumber = (
  proxyID: number,
  key: 'max_concurrency' | 'weight' | 'timeout_seconds' | 'error_circuit_threshold' | 'circuit_cooldown_seconds' | 'fallback_order',
  event: Event,
  minimum: number
) => {
  const raw = Number((event.target as HTMLInputElement).value)
  updateLane(proxyID, key, Math.max(minimum, Number.isFinite(raw) ? Math.trunc(raw) : minimum))
}

const updateLaneStrategy = (value: ProxyLaneStrategy) => emit('update:laneStrategy', value)

const selectedLabel = computed(() => selectedProxies.value.length > 0
  ? t('admin.accounts.proxyPoolSelected', { count: selectedProxies.value.length })
  : t('admin.accounts.proxyPoolEmpty'))

const toggleProxy = (id: number) => {
  const selected = new Set(props.modelValue)
  if (selected.has(id)) selected.delete(id)
  else selected.add(id)
  emit('update:modelValue', Array.from(selected))
}

watch(
  () => [props.primaryProxyId, props.proxies, props.modelValue] as const,
  () => {
    // Proxy lists are loaded asynchronously by the account page. Do not erase
    // persisted selections during the brief empty-list phase.
    if (props.proxies.length === 0) return
    const usable = new Set(selectableProxies.value.map((proxy) => proxy.id))
    const normalized = Array.from(new Set(props.modelValue)).filter((id) => usable.has(id))
    if (normalized.length !== props.modelValue.length || normalized.some((id, index) => id !== props.modelValue[index])) {
      emit('update:modelValue', normalized)
    }
  },
  { deep: true, immediate: true }
)

watch(
  () => [props.primaryProxyId, props.modelValue, props.proxies, props.accountConcurrency] as const,
  () => {
    if (props.proxies.length === 0 || laneProxies.value.length === 0) return
    // Opening an existing single-proxy account must not opt it into lane limits.
    if (props.modelValue.length === 0 && props.laneConfigs.length === 0) return
    const normalized = normalizedLaneConfigs()
    if (JSON.stringify(normalized) !== JSON.stringify(props.laneConfigs)) {
      emit('update:laneConfigs', normalized)
    }
  },
  { deep: true, immediate: true }
)
</script>

<style scoped>
.select-trigger {
  @apply flex w-full cursor-pointer items-center justify-between gap-2 rounded-xl border border-gray-200 bg-white px-4 py-2.5 text-sm text-gray-900 transition-all duration-200 hover:border-gray-300 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100 dark:hover:border-dark-500;
}

.select-trigger-disabled {
  @apply cursor-not-allowed bg-gray-100 opacity-60 dark:bg-dark-900;
}

.select-value {
  @apply flex-1 truncate text-left;
}
</style>
