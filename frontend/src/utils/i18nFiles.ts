/**
 * 应用包 `i18n/` 目录：locale 文件的「插入模板」，以及官方那条
 * 「只要有 i18n/ 就必须提供 en-US」的约定。
 *
 * 为什么单独成模块：模板是**纯文本**，而官方给的是一套固定的 section / key
 * （common / entry / install / uninstall）——从零写很容易漏掉某一节、或者把
 * `[entry]` 写成 `[entries]`，表现是应用中心里那处文案直接空白。这份骨架把键
 * 全留着、值留成空串，用户按需要填；en-US 和 zh-CN 给的是同一份（键必须一致，
 * 值各写各的），**其他语言不给**——文档只点名要求 en-US，ja-JP 之类不需要
 * 「插入模板」按钮。
 *
 * 「缺 en-US」的判据也放这里（纯函数），这样 `check:editor` 能单独跑它：
 * 算错的表现是「该提示的不提示」（打出来的包在英文环境下元数据 / 入口 / 向导
 * 文案全是空的），只在界面上看，光看代码看不出来。
 */

/**
 * locale 文件模板：官方文档里那份示例**去掉值**（只留空的双引号）。
 *
 * 键就是占位符的名字（`${common.desc}` 取 `[common]` 下的 `desc`），所以一个都
 * 不能少——少一个键，用到它的那个字段在 fnOS 侧就取不到值。
 */
export const I18N_LOCALE_TEMPLATE = `[common]
displayName=""
desc=""
changelog=""

[entry]
title=""
desc=""

[install]
stepTitle=""
label=""
helpText=""
option1Label=""
option2Label=""

[uninstall]
stepTitle=""
label=""
helpText=""
option1Label=""
option2Label=""
`

/** 给了模板的 locale（相对项目根，和编辑器里 templateOptions 用的路径同一套写法）。 */
const TEMPLATED_LOCALES = ['i18n/en-US', 'i18n/zh-CN']

/**
 * 这个路径有没有 locale 模板。
 *
 * 返回空串表示「没有通用模板」，编辑器据此不显示「插入模板」按钮（其他语言、
 * 以及 i18n 以外的任何文件都走这一支）。
 */
export function i18nTemplateFor(path: string): string {
  return TEMPLATED_LOCALES.includes(path) ? I18N_LOCALE_TEMPLATE : ''
}

/**
 * i18n 目录里缺不缺必需的 en-US。
 *
 * `entries` 是 i18n 目录的列表缓存，**undefined 表示「项目里没有这个目录」或者
 * 「还没拉过」**——两种都不报问题：前者是官方允许的「这个应用不做国际化」，后者
 * 是「还不知道」，调用方需要准确答案时（拦打包）得先 `loadDir(..., true)` 现拉一次。
 *
 * 收结构类型而不是 `FsEntry`：`utils/` 里的纯逻辑要能被检查脚本直接跑，不引 types。
 */
export function localeMissingEnUS(
  entries: readonly { name: string; dir: boolean }[] | undefined,
): boolean {
  if (!entries) return false
  // 目录名叫 en-US 不算——官方要的是那个文件。
  return !entries.some((entry) => entry.name === 'en-US' && !entry.dir)
}
