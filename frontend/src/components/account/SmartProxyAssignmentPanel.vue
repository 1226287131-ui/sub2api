<template>
  <div class="rounded-xl border border-primary-200 bg-primary-50/50 p-4 dark:border-primary-900/50 dark:bg-primary-950/20">
    <div class="flex items-start justify-between gap-4">
      <div>
        <div class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.smartProxy.title') }}
        </div>
        <p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
          {{ t('admin.accounts.smartProxy.hint') }}
        </p>
      </div>
      <input
        v-if="showEnable"
        :checked="modelValue.enabled"
        data-testid="smart-proxy-enabled"
        type="checkbox"
        class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        @change="update('enabled', ($event.target as HTMLInputElement).checked)"
      />
    </div>

    <div
      class="mt-4 space-y-3"
      :class="showEnable && !modelValue.enabled && 'pointer-events-none opacity-50'"
    >
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div>
          <label class="input-label text-xs">{{ t('admin.accounts.smartProxy.proxyCount') }}</label>
          <input
            :value="modelValue.proxy_count"
            data-testid="smart-proxy-count"
            type="number"
            min="1"
            max="32"
            class="input"
            @input="updateCount($event)"
          />
          <p class="input-hint">{{ t('admin.accounts.smartProxy.proxyCountHint') }}</p>
        </div>
        <div>
          <label class="input-label text-xs">{{ t('admin.accounts.smartProxy.lowLatencyLimit') }}</label>
          <input
            :value="modelValue.low_latency_limit"
            data-testid="smart-proxy-low-latency-limit"
            type="number"
            min="0"
            class="input"
            @input="updateLowLatencyLimit($event)"
          />
          <p class="input-hint">{{ t('admin.accounts.smartProxy.lowLatencyLimitHint') }}</p>
        </div>
      </div>

      <label class="flex items-start gap-3 text-sm text-gray-700 dark:text-gray-300">
        <input
          :checked="modelValue.test_latency"
          data-testid="smart-proxy-test-latency"
          type="checkbox"
          class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          @change="update('test_latency', ($event.target as HTMLInputElement).checked)"
        />
        <span>
          <span class="font-medium">{{ t('admin.accounts.smartProxy.testLatency') }}</span>
          <span class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.smartProxy.testLatencyHint') }}
          </span>
        </span>
      </label>

      <label class="flex items-start gap-3 text-sm text-gray-700 dark:text-gray-300">
        <input
          :checked="modelValue.prefer_low_latency"
          data-testid="smart-proxy-prefer-low-latency"
          type="checkbox"
          class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          @change="update('prefer_low_latency', ($event.target as HTMLInputElement).checked)"
        />
        <span>
          <span class="font-medium">{{ t('admin.accounts.smartProxy.preferLowLatency') }}</span>
          <span class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.smartProxy.preferLowLatencyHint') }}
          </span>
        </span>
      </label>

      <label class="flex items-start gap-3 text-sm text-gray-700 dark:text-gray-300">
        <input
          :checked="modelValue.weighted_by_load"
          data-testid="smart-proxy-weighted-load"
          type="checkbox"
          class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          @change="update('weighted_by_load', ($event.target as HTMLInputElement).checked)"
        />
        <span>
          <span class="font-medium">{{ t('admin.accounts.smartProxy.weightedByLoad') }}</span>
          <span class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.smartProxy.weightedByLoadHint') }}
          </span>
        </span>
      </label>

      <button
        v-if="showAction"
        type="button"
        data-testid="smart-proxy-apply"
        class="btn btn-primary w-full"
        :disabled="loading || (showEnable && !modelValue.enabled)"
        @click="$emit('apply')"
      >
        {{ loading ? t('admin.accounts.smartProxy.assigning') : t('admin.accounts.smartProxy.apply') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { SmartProxyAssignmentOptions } from '@/types'

const props = withDefaults(defineProps<{
  modelValue: SmartProxyAssignmentOptions
  loading?: boolean
  showAction?: boolean
  showEnable?: boolean
}>(), {
  loading: false,
  showAction: true,
  showEnable: false
})

const emit = defineEmits<{
  'update:modelValue': [value: SmartProxyAssignmentOptions]
  apply: []
}>()

const { t } = useI18n()

const update = <K extends keyof SmartProxyAssignmentOptions>(
  key: K,
  value: SmartProxyAssignmentOptions[K]
) => {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}

const updateCount = (event: Event) => {
  const value = Number((event.target as HTMLInputElement).value)
  update('proxy_count', Math.min(32, Math.max(1, Number.isFinite(value) ? Math.trunc(value) : 1)))
}

const updateLowLatencyLimit = (event: Event) => {
  const value = Number((event.target as HTMLInputElement).value)
  update('low_latency_limit', Math.max(0, Number.isFinite(value) ? Math.trunc(value) : 0))
}
</script>
