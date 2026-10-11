import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

/**
 * 界面形态（桌面 / 移动）。
 *
 * 移动形态不是「另一套页面」，而是同一批组件在窄屏下换一种排布：文件管理器固定
 * 用标签页模式、不显示文件树，项目内的文件在子页面里编辑。所以这里只需要一个
 * 「现在是哪种形态」的开关，不必再复制一份路由。
 *
 * `auto` 按视口宽度判（断点和 Tailwind 的 md 一致，< 768px 算窄屏）；另外两档是
 * 用户在设置里手动钉死的——手机横屏、或者桌面浏览器窗口拉得很窄时，自动判断
 * 不一定合用户的心意。
 */
export type LayoutMode = 'auto' | 'desktop' | 'mobile'

const STORAGE_KEY = 'fpkstudio.layout'

/** 窄屏断点，和 Tailwind 的 `md` 保持一致。 */
const narrowQuery = window.matchMedia('(max-width: 767px)')

function readMode(): LayoutMode {
  const saved = window.localStorage.getItem(STORAGE_KEY)
  return saved === 'desktop' || saved === 'mobile' ? saved : 'auto'
}

export const useLayoutStore = defineStore('layout', () => {
  const mode = ref<LayoutMode>(readMode())
  const narrow = ref(narrowQuery.matches)

  const isMobile = computed(
    () => mode.value === 'mobile' || (mode.value === 'auto' && narrow.value),
  )

  function setMode(next: LayoutMode) {
    mode.value = next
    window.localStorage.setItem(STORAGE_KEY, next)
  }

  /** 挂上视口监听。只该在应用启动时调一次。 */
  function init() {
    narrowQuery.addEventListener('change', (event) => {
      narrow.value = event.matches
    })
  }

  return { mode, narrow, isMobile, setMode, init }
})
