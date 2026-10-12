import { createI18n } from 'vue-i18n'
import { reactive } from 'vue'
import elementZh from 'element-plus/es/locale/lang/zh-cn'
import elementEn from 'element-plus/es/locale/lang/en'
import zhCN from './zh-CN'
import enUS from './en-US'

export type AppLocale = 'zh-CN' | 'en-US'
/** 界面语言的三档：两门语言 + 跟随系统。 */
export type LocaleMode = AppLocale | 'system'

/** 语言偏好记在本地（和主题、显示方式一样），不进后端状态文件。 */
const STORAGE_KEY = 'fpkstudio.locale'

/** 系统语言：只看浏览器 / 系统的 navigator.language，非中文一律英文。 */
export function systemLocale(): AppLocale {
  const nav = navigator.language || 'zh-CN'
  return nav.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US'
}

export function readLocaleMode(): LocaleMode {
  const saved = window.localStorage.getItem(STORAGE_KEY)
  return saved === 'zh-CN' || saved === 'en-US' ? saved : 'system'
}

export function writeLocaleMode(mode: LocaleMode) {
  window.localStorage.setItem(STORAGE_KEY, mode)
}

export function resolveLocale(mode: LocaleMode): AppLocale {
  return mode === 'system' ? systemLocale() : mode
}

/** 启动时的语言：本地存过就用存的那门，否则跟随系统（两处用的是同一份判断）。 */
export const currentLocale = resolveLocale(readLocaleMode())

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: currentLocale,
  fallbackLocale: 'en-US',
  missingWarn: false,
  fallbackWarn: false,
  messages: { 'zh-CN': zhCN, 'en-US': enUS },
})

/**
 * Element Plus 的全局配置（`main.ts` 把这一整个对象交给 `app.use`）。
 *
 * **必须是 reactive 的这一份**，不能在 `app.use` 里塞一个字面量：切语言时改它的
 * `locale`，组件树和 `ElMessage` / `ElMessageBox` 这类命令式 API 读的是同一个对象；
 * 塞字面量的话只有页面上的文案会换，组件库自己的文案还是旧语言。
 */
export const elementOptions = reactive({
  locale: currentLocale === 'zh-CN' ? elementZh : elementEn,
  size: 'small' as const,
})

/**
 * 切到某门语言：vue-i18n、`html lang`、Element Plus 三处一起换。
 *
 * `html lang` 必须跟着走——文档语言和实际语言不一致时，屏幕阅读器和翻译插件会认错。
 */
export function applyLocale(locale: AppLocale) {
  i18n.global.locale.value = locale
  document.documentElement.lang = locale
  elementOptions.locale = locale === 'zh-CN' ? elementZh : elementEn
}
