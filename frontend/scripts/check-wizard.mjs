/**
 * 校验向导文件的解析与回写（src/utils/wizard.ts）。
 *
 *   npm run check:wizard
 *
 * 这段逻辑会**重写用户项目里的文件**，所以盯三件事：
 *  1. 真实的 fpk 向导（含 ${i18n} 占位符、tips、radio 选项）解析后原样回写；
 *  2. 七种字段类型、四种校验规则都能往返，且类型专属的键不会串（开关存布尔、
 *     多选存数组）；
 *  3. 表单里没有的键（fnOS 以后加的字段）原样保留，不会保存一次就丢；
 *  4. step 的键序是 stepTitle 在前、items 在后（源码是给人读的）。
 */
import { dumpWizard, parseWizard, nextWizardField, newWizardItem, newWizardRule } from '../src/utils/wizard.ts'

let failed = 0
const check = (name, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want)
  if (!ok) failed++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(got)}\n     want ${JSON.stringify(want)}`}`)
}
const eq = (name, got, want) => check(name, got, want)

// ---- 1. 真实 fpk 向导 ----
const realInstall = JSON.stringify(
  [
    {
      stepTitle: '${install.stepTitle}',
      items: [
        { type: 'tips', helpText: '${install.helpText}' },
        {
          type: 'text',
          field: 'wizard_fnos_http_port',
          label: '${install.label}',
          initValue: '5666',
          rules: [{ required: true, message: '${install.message}' }],
          helpText: '${install.helpText1}',
        },
      ],
    },
  ],
  null,
  2,
)
const realUninstall = JSON.stringify(
  [
    {
      stepTitle: '${uninstall.stepTitle}',
      items: [
        { type: 'tips', helpText: '${uninstall.helpText}' },
        {
          type: 'radio',
          field: 'wizard_data_action',
          label: '${uninstall.label}',
          initValue: 'keep',
          options: [
            { label: '${uninstall.optionLabel}', value: 'keep' },
            { label: '${uninstall.optionLabel1}', value: 'delete' },
          ],
          rules: [{ required: true, message: '${uninstall.message}' }],
        },
      ],
    },
  ],
  null,
  2,
)

for (const [name, text] of [
  ['install', realInstall],
  ['uninstall', realUninstall],
]) {
  const model = parseWizard(text)
  eq(`${name}：解析无错误`, model.error, '')
  // 回写结果再解析一次要完全一致（键序固定，所以能直接比文本）
  eq(`${name}：回写稳定`, dumpWizard(parseWizard(dumpWizard(model))), dumpWizard(model))
}

// ---- 2. 七种类型 + 四种校验 ----
const step = {
  stepTitle: '欢迎使用',
  extra: {},
  items: [
    { ...newWizardItem('wizard_field', '示例项'), rules: [{ ...newWizardRule('required'), message: '必填' }] },
    { ...newWizardItem('pwd', '密码'), type: 'password', initValue: 'secret' },
    { ...newWizardItem('mode', '模式'), type: 'radio', initValue: '1', options: [{ label: '一', value: '1' }] },
    { ...newWizardItem('langs', '语言'), type: 'checkbox', initValues: ['zh', 'en'] },
    { ...newWizardItem('level', '等级'), type: 'select', initValue: '2', options: [{ label: '高', value: '2' }] },
    { ...newWizardItem('enable', '启用'), type: 'switch', initValue: 'true' },
    { ...newWizardItem('', '说明'), type: 'tips', helpText: '请看说明' },
  ],
}
step.items[0].rules.push(
  { ...newWizardRule('min'), message: '范围' },
  { ...newWizardRule('len'), len: 6, message: '长度' },
  { ...newWizardRule('pattern'), message: '格式' },
)
// 表单不认识的键
step.extra = { stepIcon: 'star' }
step.items[0].extra = { placeholder: '请输入' }
step.items[0].rules[0].extra = { trigger: 'blur' }

const text = dumpWizard({ steps: [step], error: '' })
const rows = JSON.parse(text)
const parsed = parseWizard(text)
const byField = (field) => parsed.steps[0].items.find((item) => item.field === field)
const rawItem = (field) => rows[0].items.find((item) => item.field === field)

eq('顶层是数组', Array.isArray(rows), true)
// 键序：stepTitle 必须排在 items 前面。纯字母序是 items 在前，读源码时要先翻完
// 一整屏表单项才知道这一步叫什么。
eq(
  'step 的键序：stepTitle 在 items 之前（不认识的键排在后面）',
  Object.keys(rows[0]),
  ['stepTitle', 'items', 'stepIcon'],
)
check(
  'step 的源码里 stepTitle 出现在 items 之前',
  text.indexOf('"stepTitle"') < text.indexOf('"items"'),
  true,
)
eq('不认识的 step 键保留', parsed.steps[0].extra, { stepIcon: 'star' })
eq('不认识的 item 键保留', byField('wizard_field').extra, { placeholder: '请输入' })
eq('不认识的 rule 键保留', byField('wizard_field').rules[0].extra, { trigger: 'blur' })
eq('password 默认值', byField('pwd').initValue, 'secret')
eq('checkbox 存数组', rawItem('langs').initValue, ['zh', 'en'])
eq('switch 存布尔', rawItem('enable').initValue, true)
eq('select 默认值', byField('level').initValue, '2')
// tips 只写 type / label / helpText（没有字段名 / 默认值 / 校验）。文件里带着 label 的
// 要原样写回（表单里不再露出它，但也不该把它删掉）。
eq(
  'tips 只写 type / label / helpText',
  Object.keys(rows[0].items.find((item) => item.type === 'tips')),
  ['helpText', 'label', 'type'],
)
eq('四条规则都在', byField('wizard_field').rules.map((r) => r.type), ['required', 'min', 'len', 'pattern'])
eq('范围规则', rawItem('wizard_field').rules[1], { max: 100, message: '范围', min: 1 })
eq('长度规则', rawItem('wizard_field').rules[2], { len: 6, message: '长度' })
eq('正则规则', rawItem('wizard_field').rules[3], { message: '格式', pattern: '^[0-9]+$' })
eq('二次解析稳定', dumpWizard(parseWizard(text)), text)

// ---- 3. 空 / 坏文件 ----
eq('空文件不报错', parseWizard('').steps.length, 0)
eq('空数组不报错', parseWizard('[]').steps.length, 0)
check('坏 JSON 报错', parseWizard('{oops').error !== '', true)
eq('顶层不是数组报错', parseWizard('{"a":1}').error, 'top level is not an array')

// 提示文字类字段在表单里只露出「辅助文本」（字段标签 / 字段名 / 默认值都藏起来，它的
// 正文就是辅助文本）。但文件里已经写着的 label 不能因为保存一次就丢——dump 仍原样回写，
// 这里钉住「保存一次不掉键、改了也确实进得去」。
{
  const tipsText = JSON.stringify(
    [{ stepTitle: 's', items: [{ type: 'tips', label: '标题', helpText: '说明' }] }],
    null,
    2,
  )
  eq('tips 的 label 保留', JSON.parse(dumpWizard(parseWizard(tipsText)))[0].items[0].label, '标题')
  check(
    '只改 tips 的 label 也会让内容变化',
    dumpWizard(parseWizard(tipsText)) !== dumpWizard(parseWizard(tipsText.replace('标题', '改过'))),
    true,
  )
}

// ---- 4. 默认字段名不重复（它同时是脚本变量名）----
const used = { steps: [{ ...step, items: [newWizardItem('wizard_field', 'a'), newWizardItem('wizard_field_2', 'b')] }], error: '' }
eq('跳过已用字段名', nextWizardField(used), 'wizard_field_3')

console.log(failed ? `\n${failed} 项不通过` : '\n向导解析 / 回写检查全部通过')
process.exit(failed ? 1 : 0)
