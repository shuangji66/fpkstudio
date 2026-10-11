/**
 * 校验两份语言包：每条消息都能被 vue-i18n 编译，且两份的键完全一致。
 *
 * 为什么需要它：i18n 消息里的 `{` 是插值语法。写了一句带 `${...}` 的提示
 * （想表达脚本里的变量）就会让这条消息在 `t()` 时抛 SyntaxError——而且是在
 * 渲染到那个组件时才抛，表现是「弹窗整体画不出来」，从控制台之外很难定位。
 *
 *   node scripts/check-i18n.mjs      # 或 npm run check:i18n
 *
 * 直接 import .ts（Node 23.6+ 默认开启类型剥离），所以不需要先构建。
 */
import { createI18n } from 'vue-i18n'
import zhCN from '../src/i18n/zh-CN.ts'
import enUS from '../src/i18n/en-US.ts'

function leaves(value, prefix = '') {
  const out = []
  for (const [key, child] of Object.entries(value)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (child && typeof child === 'object') out.push(...leaves(child, path))
    else out.push(path)
  }
  return out
}

let broken = 0
for (const [locale, messages] of [
  ['zh-CN', zhCN],
  ['en-US', enUS],
]) {
  const i18n = createI18n({
    legacy: false,
    locale,
    fallbackLocale: 'en-US',
    messages: { 'zh-CN': zhCN, 'en-US': enUS },
  })
  for (const key of leaves(messages)) {
    try {
      i18n.global.t(key)
    } catch (e) {
      broken += 1
      console.error(`[${locale}] ${key}: ${String(e.message).split('\n')[0]}`)
    }
  }
}

const zhKeys = new Set(leaves(zhCN))
const enKeys = new Set(leaves(enUS))
const missingEn = [...zhKeys].filter((key) => !enKeys.has(key))
const missingZh = [...enKeys].filter((key) => !zhKeys.has(key))

for (const key of missingEn) console.error(`缺 en-US: ${key}`)
for (const key of missingZh) console.error(`缺 zh-CN: ${key}`)

if (broken || missingEn.length || missingZh.length) {
  console.error(`\n检查失败：${broken} 条消息无法编译，${missingEn.length + missingZh.length} 个键缺失`)
  process.exit(1)
}
console.log(`i18n 检查通过：${zhKeys.size} 个键，两份语言包一致且都能编译`)
