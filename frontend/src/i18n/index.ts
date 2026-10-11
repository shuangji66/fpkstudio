import { createI18n } from 'vue-i18n'
import elementZh from 'element-plus/es/locale/lang/zh-cn'
import elementEn from 'element-plus/es/locale/lang/en'
import zhCN from './zh-CN'
import enUS from './en-US'

function detectLocale(): string {
  // 与服务端/系统保持一致即可，优先看浏览器；这里不做持久化，保持无状态。
  const nav = navigator.language || 'zh-CN'
  return nav.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US'
}

export const currentLocale = detectLocale()

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: currentLocale,
  fallbackLocale: 'en-US',
  missingWarn: false,
  fallbackWarn: false,
  messages: { 'zh-CN': zhCN, 'en-US': enUS },
})

export const elementLocale = currentLocale === 'zh-CN' ? elementZh : elementEn
