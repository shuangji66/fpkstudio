import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import {
  applyLocale,
  readLocaleMode,
  systemLocale,
  writeLocaleMode,
  type AppLocale,
  type LocaleMode,
} from '@/i18n'

/**
 * 界面语言（中文 / English / 跟随系统）。
 *
 * 三档和主题那套一样，也是记在本地、不进后端状态文件。`i18n` 创建时已经用同一份
 * 判断取过初始语言了，这里负责后续的切换，以及把 `html lang` 和 Element Plus 的
 * 语言包对齐（见 `i18n/index.ts` 的 `applyLocale`）。
 */
export const useLocaleStore = defineStore('locale', () => {
  const mode = ref<LocaleMode>(readLocaleMode())

  /** 「跟随系统」那一档解析出来的语言。系统语言变了不通知，所以每次用到时重新读。 */
  const system = ref<AppLocale>(systemLocale())
  const resolved = computed<AppLocale>(() => (mode.value === 'system' ? system.value : mode.value))

  function setMode(next: LocaleMode) {
    mode.value = next
    writeLocaleMode(next)
    system.value = systemLocale()
  }

  // immediate：应用启动时就应用一次（`html lang` 和 Element Plus 的语言包要靠它对齐）
  watch(resolved, (next) => applyLocale(next), { immediate: true })

  return { mode, system, resolved, setMode }
})
