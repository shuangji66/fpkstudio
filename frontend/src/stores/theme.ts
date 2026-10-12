import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { hostTheme, onHostThemeChange } from '@/trim/sdk'

/**
 * 主题（浅色 / 深色 / 跟随系统）。
 *
 * 「跟随系统」在宿主里跟的是**飞牛的主题**（`os/theme`，用户可能在系统设置里
 * 切了暗黑模式），在独立浏览器页面里跟的是 `prefers-color-scheme`。这两条都
 * 要能实时响应，所以各自挂了监听。
 *
 * 深色不是换一套色号就完事：Tailwind 的 dark 变体按 html.dark 生效，
 * Element Plus 的暗色变量也认这个类，两边靠同一个类对齐。
 */
export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'fpkstudio.theme'

/** 系统浅深色偏好。和「显示方式」一样记在本地，不进后端状态文件。 */
const darkQuery = window.matchMedia('(prefers-color-scheme: dark)')

function readMode(): ThemeMode {
  const saved = window.localStorage.getItem(STORAGE_KEY)
  return saved === 'light' || saved === 'dark' ? saved : 'system'
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(readMode())
  const systemDark = ref(darkQuery.matches)
  /** 宿主主题；读不到（独立浏览器）时为空，交给 prefers-color-scheme。 */
  const hostThemeValue = ref<'light' | 'dark' | ''>('')

  const resolved = computed<'light' | 'dark'>(() => {
    if (mode.value !== 'system') return mode.value
    return hostThemeValue.value || (systemDark.value ? 'dark' : 'light')
  })

  function apply() {
    const root = document.documentElement
    root.classList.toggle('dark', resolved.value === 'dark')
    root.dataset.theme = resolved.value
    // 让原生控件（滚动条、日期选择、下拉）也跟着切，不然深色页面里会冒出白条。
    root.style.colorScheme = resolved.value
  }

  function setMode(next: ThemeMode) {
    mode.value = next
    window.localStorage.setItem(STORAGE_KEY, next)
    apply()
  }

  /** 挂上监听并应用一次。只该在应用启动时调一次。 */
  async function init() {
    darkQuery.addEventListener('change', (event) => {
      systemDark.value = event.matches
      apply()
    })

    // 先按「本地设置 + 系统偏好」应用一次：宿主主题是异步读的，读得慢、或者压根读
    // 不到（宿主桥没就绪时 hostTheme 会等到超时）都不该让首屏停在「没有主题」上。
    apply()

    hostThemeValue.value = (await hostTheme()) ?? ''
    void onHostThemeChange((theme) => {
      hostThemeValue.value = theme
      apply()
    })

    // 宿主主题拿到之后（可能是空）再应用一次，这样「跟随系统」那一档才真的生效。
    apply()
  }

  return { mode, resolved, systemDark, hostThemeValue, setMode, init, apply }
})
