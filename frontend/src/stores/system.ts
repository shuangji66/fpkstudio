import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { getSystemInfo } from '@/api/endpoints'
import type { SystemInfo } from '@/api/types'

export const useSystemStore = defineStore('system', () => {
  const info = ref<SystemInfo | null>(null)
  const loading = ref(false)
  const error = ref('')

  async function load(force = false) {
    if (loading.value) return
    if (info.value && !force) return
    loading.value = true
    error.value = ''
    try {
      info.value = await getSystemInfo()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  const fnpackReady = computed(() => info.value?.fnpack.ok === true)
  const outputDir = computed(() => info.value?.outputDir ?? '')

  return { info, loading, error, load, fnpackReady, outputDir }
})
