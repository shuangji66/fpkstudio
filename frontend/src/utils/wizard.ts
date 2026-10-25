/**
 * 向导文件（wizard/install、uninstall、upgrade、config）的解析与回写。
 *
 * 文件是 fnpack 的 JSON 数组：每个 step 有 stepTitle 和 items，item 靠 type 区分
 * 表单控件，rules 里放校验。和 `manifest.ts` 一样，解析与序列化放在这里，
 * 组件只负责把模型接到表单上——这样这段「会重写用户文件」的逻辑能单独验证。
 *
 * 两点刻意做得比 fnpackup 稳：
 *  1. 校验规则按 required / min,max / len / pattern 这几个键判类型，并且支持一个
 *     字段挂多条规则（fnpackup 的界面只能加一条）。
 *  2. 不认识的键（fnOS 以后加的字段）原样留在 extra 里，保存时写回去，不会因为
 *     「表单里没有这个控件」就把它删掉。
 */

import { stringifyOrdered } from './json'

export interface WizardOption {
  label: string
  value: string
}

/** 一条校验规则。四种类型共用一个扁平结构，回写时只取当前类型用得上的键。 */
export interface WizardRule {
  type: string
  required: boolean
  min: number
  max: number
  len: number
  pattern: string
  message: string
  /** 认识之外的键，原样回写。 */
  extra: Record<string, unknown>
}

export interface WizardItem {
  type: string
  field: string
  label: string
  /** text / password / radio / select / switch 的默认值。 */
  initValue: string
  /** checkbox 的默认值（多选）。 */
  initValues: string[]
  helpText: string
  options: WizardOption[]
  rules: WizardRule[]
  /** 表单没覆盖到的键，原样回写。 */
  extra: Record<string, unknown>
}

export interface WizardStep {
  stepTitle: string
  items: WizardItem[]
  extra: Record<string, unknown>
}

export interface WizardModel {
  steps: WizardStep[]
  error: string
}

/** 字段类型，value 就是写进 JSON 的 type。 */
export const WIZARD_TYPES = ['text', 'password', 'radio', 'checkbox', 'select', 'switch', 'tips']

/** 校验类型：靠 required / min,max / len / pattern 这几个键区分。 */
export const WIZARD_RULE_TYPES = ['required', 'min', 'len', 'pattern']

const KNOWN_ITEM_KEYS = ['type', 'field', 'label', 'initValue', 'helpText', 'options', 'rules']
const KNOWN_RULE_KEYS = ['required', 'min', 'max', 'len', 'pattern', 'message']
const KNOWN_STEP_KEYS = ['stepTitle', 'items']

/** 正则校验的默认值，新建时直接可用。 */
export const DEFAULT_PATTERN = '^[0-9]+$'

/**
 * 正则校验的常用预制规则（界面上是「常用规则」下拉，选项只显示描述，选中后把正则
 * 填进输入框，之后照旧能手改）。
 *
 * 这里只放「i18n 键 + 正则」：描述走 `wizard.presets.*`，util 里不引 vue-i18n
 * （check:wizard 直接跑这个文件）。正则必须是 **Go / RE2 也认**的写法——fnOS 后端
 * 用 Go 校验，`(?=...)`、反向引用这类在那里编译不过；「密码必须同时有字母和数字」
 * 就是因此做不了，只能限字符集。
 */
export const WIZARD_PATTERN_PRESETS: { key: string; pattern: string }[] = [
  {
    key: 'port',
    // 1025–65535：1024 以下是特权端口，普通应用绑不上
    pattern:
      '^(102[5-9]|10[3-9]\\d|1[1-9]\\d\\d|[2-9]\\d{3}|[1-5]\\d{4}|6[0-4]\\d{3}|65[0-4]\\d{2}|655[0-2]\\d|6553[0-5])$',
  },
  {
    key: 'ipv4',
    pattern: '^((25[0-5]|2[0-4]\\d|1\\d\\d|[1-9]?\\d)\\.){3}(25[0-5]|2[0-4]\\d|1\\d\\d|[1-9]?\\d)$',
  },
  {
    key: 'host',
    // 域名 / 主机名：每段 1–63 字符，不以连字符开头结尾
    pattern:
      '^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$',
  },
  { key: 'url', pattern: '^https?://[^\\s]+$' },
  {
    key: 'password',
    // 字母 / 数字 / 常见 ASCII 标点，不限长度（复杂度类断言 RE2 写不了）
    pattern: '^[A-Za-z0-9!@#$%^&*()_+\\-=\\[\\]{};:\'",.<>/?\\\\|`~]+$',
  },
]

function ruleTypeOf(raw: Record<string, unknown>): string {
  if ('min' in raw || 'max' in raw) return 'min'
  if ('len' in raw) return 'len'
  if ('pattern' in raw) return 'pattern'
  return 'required'
}

/** 挑出「这个编辑器不认识的键」，保存时原样写回。 */
function pickExtra(raw: Record<string, unknown>, known: string[]): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(raw)) {
    if (!known.includes(key)) out[key] = value
  }
  return out
}

export function parseWizard(text: string): WizardModel {
  const trimmed = text.trim()
  // 空文件 / 空数组都当成「还没开始写」，不报错
  if (!trimmed || trimmed === '[]') return { steps: [], error: '' }
  try {
    const raw = JSON.parse(trimmed) as unknown
    // 这句会被插进 editor.formatWizard 的 {detail} 里，和技术性报错（JSON.parse 的
    // 英文原话）同一性质，所以写英文：硬编码中文在英文界面里是「中文夹在英文句子里」。
    if (!Array.isArray(raw)) return { steps: [], error: 'top level is not an array' }

    const steps: WizardStep[] = raw.map((step) => {
      const source = (step ?? {}) as Record<string, unknown>
      const items = Array.isArray(source.items) ? (source.items as Record<string, unknown>[]) : []
      return {
        stepTitle: String(source.stepTitle ?? ''),
        extra: pickExtra(source, KNOWN_STEP_KEYS),
        items: items.map((item) => {
          const itemExtra = pickExtra(item, KNOWN_ITEM_KEYS)
          const rawRules = Array.isArray(item.rules) ? (item.rules as Record<string, unknown>[]) : []
          return {
            type: String(item.type ?? 'text'),
            field: String(item.field ?? ''),
            label: String(item.label ?? ''),
            initValue: item.initValue === undefined ? '' : String(item.initValue),
            initValues: Array.isArray(item.initValue)
              ? item.initValue.map((v) => String(v))
              : item.initValue === undefined || item.initValue === ''
                ? []
                : [String(item.initValue)],
            helpText: String(item.helpText ?? ''),
            options: Array.isArray(item.options)
              ? (item.options as Record<string, unknown>[]).map((o) => ({
                  label: String(o.label ?? ''),
                  value: String(o.value ?? ''),
                }))
              : [],
            rules: rawRules.map((rule) => ({
              type: ruleTypeOf(rule),
              required: rule.required !== false,
              min: Number(rule.min ?? 0),
              max: Number(rule.max ?? 0),
              len: Number(rule.len ?? 0),
              pattern: String(rule.pattern ?? ''),
              message: String(rule.message ?? ''),
              extra: pickExtra(rule, KNOWN_RULE_KEYS),
            })),
            extra: itemExtra,
          }
        }),
      }
    })
    return { steps, error: '' }
  } catch (e) {
    return { steps: [], error: e instanceof Error ? e.message : String(e) }
  }
}

export function dumpWizard(model: WizardModel): string {
  const steps = model.steps.map((step) => {
    const items: Record<string, unknown>[] = step.items.map((item) => {
      const out: Record<string, unknown> = { ...item.extra, type: item.type }
      // tips 只有一段说明文字，没有字段名 / 默认值 / 校验。
      // label（界面上是「显示文本」）必须一起写回：表单里它是可编辑的，不写回时
      // 序列化结果完全不变——改它连「未保存」都不会亮、保存按钮也不亮，切个布局
      // 再回来字就没了。「插入模板」给的那两份 tips 也带着 label。
      if (item.type === 'tips') {
        if (item.label) out.label = item.label
        if (item.helpText) out.helpText = item.helpText
        return out
      }
      if (item.field) out.field = item.field
      if (item.label) out.label = item.label

      if (item.type === 'switch') {
        out.initValue = item.initValue === 'true' || item.initValue === '1'
      } else if (item.type === 'checkbox') {
        out.initValue = item.initValues
      } else if (item.initValue !== '') {
        out.initValue = item.initValue
      }
      if (item.helpText) out.helpText = item.helpText
      if (item.options.length) {
        out.options = item.options.map((o) => ({ label: o.label, value: o.value }))
      }

      const rules = item.rules.map((rule) => {
        // 提示文案留空时兜到显示文本上：fnOS 会把 message 原样显示给用户
        const message = rule.message || item.label || item.field
        const base = { ...rule.extra }
        switch (rule.type) {
          case 'min':
            return { ...base, min: rule.min, max: rule.max, message }
          case 'len':
            return { ...base, len: rule.len, message }
          case 'pattern':
            return { ...base, pattern: rule.pattern, message }
          default:
            return { ...base, required: rule.required !== false, message }
        }
      })
      if (rules.length) out.rules = rules

      return out
    })
    const out: Record<string, unknown> = { ...step.extra }
    if (step.stepTitle) out.stepTitle = step.stepTitle
    if (items.length) out.items = items
    return out
  })
  // stepTitle 排在 items 前面：纯字母序是 items 在前，读源码时得先翻完一整屏
  // 表单项才知道这一步叫什么（见 stringifyOrdered 的 first 参数）。
  return stringifyOrdered(steps, ['stepTitle', 'items'])
}

/** 新建字段的骨架。字段名与显示文本由调用方给（要跟着界面语言走）。 */
export function newWizardItem(field: string, label: string): WizardItem {
  return {
    type: 'text',
    field,
    label,
    initValue: '',
    initValues: [],
    helpText: '',
    options: [],
    rules: [],
    extra: {},
  }
}

/** 新建一条校验规则；提示文案由调用方按语言补上。 */
export function newWizardRule(type: string): WizardRule {
  return {
    type,
    required: true,
    min: 1,
    max: 100,
    len: 10,
    pattern: type === 'pattern' ? DEFAULT_PATTERN : '',
    message: '',
    extra: {},
  }
}

export function newWizardStep(stepTitle: string, item: WizardItem): WizardStep {
  return { stepTitle, items: [item], extra: {} }
}

/**
 * 默认字段名。
 *
 * 字段名同时是脚本里的变量名，重名会互相覆盖，所以这里跳过已经用过的名字：
 * wizard_field、wizard_field_2、wizard_field_3…
 */
export function nextWizardField(model: WizardModel, base = 'wizard_field'): string {
  const used = new Set<string>()
  for (const step of model.steps) {
    for (const item of step.items) if (item.field) used.add(item.field)
  }
  if (!used.has(base)) return base
  for (let i = 2; ; i += 1) {
    const name = `${base}_${i}`
    if (!used.has(name)) return name
  }
}
