<template>
  <div class="min-w-[260px] space-y-1.5" data-testid="proxy-lanes-cell">
    <div v-if="lanes.length === 0" class="text-sm text-gray-400 dark:text-gray-500">-</div>
    <div
      v-for="lane in lanes"
      :key="lane.proxy_id"
      class="rounded-lg border px-2.5 py-2"
      :class="laneClasses(lane)"
    >
      <div class="flex items-center justify-between gap-3">
        <div class="min-w-0 flex items-center gap-1.5">
          <span class="h-2 w-2 shrink-0 rounded-full" :class="laneDotClass(lane)" />
          <span class="truncate text-xs font-medium" :title="lane.name">{{ lane.name }}</span>
          <span v-if="lane.primary" class="rounded bg-blue-100 px-1 py-0.5 text-[10px] text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
            {{ t('admin.accounts.proxyLanes.primary') }}
          </span>
        </div>
        <span class="shrink-0 font-mono text-xs font-semibold">
          {{ lane.current_concurrency }}/{{ lane.max_concurrency }}
        </span>
      </div>
      <div class="mt-1.5 h-1 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-500">
        <div class="h-full rounded-full transition-all" :class="barClass(lane)" :style="{ width: `${lanePercent(lane)}%` }" />
      </div>
      <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[10px] text-gray-500 dark:text-gray-400">
        <span>{{ laneStatus(lane) }}</span>
        <span v-if="proxyFor(lane.proxy_id)?.latency_ms != null">{{ proxyFor(lane.proxy_id)?.latency_ms }}ms</span>
        <span v-if="lane.weight !== 1">W{{ lane.weight }}</span>
        <span v-if="lane.timeout_seconds > 0">{{ lane.timeout_seconds }}s</span>
        <span v-if="lane.expires_at">{{ t('admin.accounts.proxyLanes.expires', { time: formatDateTime(lane.expires_at) }) }}</span>
      </div>
    </div>
    <div v-if="lanes.length > 1" class="flex items-center justify-between px-1 text-[10px] text-gray-400">
      <span>{{ strategyLabel }}</span>
      <span>{{ totalCurrent }}/{{ totalMax }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, Proxy, ProxyLaneStatus } from '@/types'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  account: Account
  proxies?: Proxy[]
}>()

const { t } = useI18n()
const proxyMap = computed(() => new Map((props.proxies || []).map((proxy) => [proxy.id, proxy])))
const proxyFor = (id: number) => proxyMap.value.get(id)
const lanes = computed(() => props.account.proxy_lanes || [])
const totalCurrent = computed(() => lanes.value.reduce((sum, lane) => sum + lane.current_concurrency, 0))
const totalMax = computed(() => lanes.value.reduce((sum, lane) => sum + (lane.enabled ? lane.max_concurrency : 0), 0))
const strategyLabel = computed(() => t(`admin.accounts.proxyLanes.strategy.${props.account.proxy_lane_strategy || 'round_robin'}`))

const lanePercent = (lane: ProxyLaneStatus) => lane.max_concurrency > 0
  ? Math.min(100, Math.round((lane.current_concurrency / lane.max_concurrency) * 100))
  : 0

const laneStatus = (lane: ProxyLaneStatus) => {
  if (!lane.enabled) return t('admin.accounts.proxyLanes.disabled')
  if (lane.circuit_open_until) return t('admin.accounts.proxyLanes.circuitOpen')
  if (!lane.healthy) return t('admin.accounts.proxyLanes.unavailable')
  if (lane.current_concurrency >= lane.max_concurrency) return t('admin.accounts.proxyLanes.full')
  return t('admin.accounts.proxyLanes.ready')
}

const laneClasses = (lane: ProxyLaneStatus) => {
  if (!lane.enabled || !lane.healthy) return 'border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400'
  if (lane.current_concurrency >= lane.max_concurrency) return 'border-red-200 bg-red-50 text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-300'
  if (lane.current_concurrency > 0) return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-300'
  return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-950/20 dark:text-emerald-300'
}

const laneDotClass = (lane: ProxyLaneStatus) => {
  if (!lane.enabled || !lane.healthy) return 'bg-gray-400'
  if (lane.current_concurrency >= lane.max_concurrency) return 'bg-red-500'
  if (lane.current_concurrency > 0) return 'bg-amber-500'
  return 'bg-emerald-500'
}

const barClass = (lane: ProxyLaneStatus) => {
  if (!lane.enabled || !lane.healthy) return 'bg-gray-400'
  if (lane.current_concurrency >= lane.max_concurrency) return 'bg-red-500'
  if (lane.current_concurrency > 0) return 'bg-amber-500'
  return 'bg-emerald-500'
}
</script>
