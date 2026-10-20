/**
 * 校验表单编辑器的「未改动」判据（src/components/editor/useEditor.ts），以及
 * 「插入模板」的文本（src/utils/lifecycle.ts、src/utils/i18nFiles.ts）。
 *
 *   npm run check:editor
 *
 * 表单类编辑器的回写会把格式归一（JSON 的缩进与键序、manifest 补默认键），所以
 * **磁盘原文 ≠ 回写结果**是常态。脏标记如果拿原文当基线，一份没编辑过的脚手架文件
 * 就会在挂载那一刻算「脏」——表单视图看不出来，一按「源码」却显示「未保存」。
 * 这条规则一旦被改回原文基线，界面上的表现是「什么都没改就说有未保存的修改」，
 * 编译期不会报错，所以在这里钉住：基线是「原文走一遍解析 + 回写」的结果。
 *
 * 另一半（切视图时表单干净就交磁盘原文，见 EditorHost.vue 的 watch(mode)）在组件里，
 * 只能按「怎么真跑起来」手点，不在这里。
 */
import { nextTick, reactive } from 'vue'
import { useEditor } from '../src/components/editor/useEditor.ts'
import { handoffText } from '../src/utils/handoff.ts'
import {
  I18N_LOCALE_TEMPLATE,
  i18nTemplateFor,
  localeMissingEnUS,
} from '../src/utils/i18nFiles.ts'
import {
  MAIN_DOCKER_SCRIPT_TEMPLATE,
  MAIN_NO_PROCESS_TEMPLATE,
  MAIN_SCRIPT_TEMPLATE,
  mainTemplateOptions,
} from '../src/utils/lifecycle.ts'

let failed = 0
const check = (name, got, want) => {
  const ok = JSON.stringify(got) === JSON.stringify(want)
  if (!ok) failed++
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${name}${ok ? '' : `\n     got  ${JSON.stringify(got)}\n     want ${JSON.stringify(want)}`}`)
}

// fnpack 生成的 config/privilege 就是 4 空格缩进、结尾不带换行的；回写按 json.ts
// 的约定输出 2 空格 + 结尾换行。
const DISK = '{\n    "defaults": {\n        "run-as": "package"\n    }\n}'

const load = (text) => JSON.parse(text || '{}')
const dump = (model) => `${JSON.stringify(model, null, 2)}\n`

/** 造一个编辑器：props 必须是响应式的，watch 才看得到 origin 的变化。 */
function makeEditor(disk = DISK) {
  const events = []
  const props = reactive({ path: 'config/privilege', origin: disk, original: disk })
  const { model, api } = useEditor(props, (_event, value) => events.push(value), load, dump)
  return { events, model, api, props }
}

// ---- 1. 没编辑过的脚手架文件是干净的 ----
{
  const { events, api } = makeEditor()
  check('没编辑过就不脏', api.isDirty(), false)
  check('脏标记没被报出去', events, [])
  // 回写结果和磁盘原文不同（缩进 / 结尾换行被归一），这正是不能拿原文当基线的原因
  check('回写结果确实不是原文', api.getContent().content === DISK, false)
}

// ---- 2. 改了就脏，改回来自己干净 ----
{
  const { events, model, api } = makeEditor()
  model.value.defaults['run-as'] = 'root'
  await nextTick()
  check('改了就是脏', api.isDirty(), true)
  check('脏标记报出去了', events, [true])

  model.value.defaults['run-as'] = 'package'
  await nextTick()
  check('改回原值就不脏了', api.isDirty(), false)
  check('干净也报出去（按钮要跟着灭）', events, [true, false])
}

// ---- 3. 还原 / 保存换了 origin 之后，基线跟着重算 ----
{
  const { events, api, props } = makeEditor()
  // 保存：写回去的就是回写结果，它现在是磁盘内容
  const saved = api.getContent().content
  props.origin = saved
  props.original = saved
  await nextTick()
  check('保存后是干净的', api.isDirty(), false)
  check('保存后仍然干净（基线重算过）', events[events.length - 1], false)

  // 还原：磁盘上那份又变回 4 空格，基线要跟着回去
  props.origin = DISK
  props.original = DISK
  await nextTick()
  check('还原回磁盘原文后不脏', api.isDirty(), false)
}

// ---- 4. 解析失败（降级成空模型）时不要把空模型当成用户的改动 ----
{
  // 向导解析失败时 load 返回空模型，回写是一份空数组；基线同样走这条路，
  // 所以表单视图是干净的，切到源码视图看到的是磁盘上那份坏文本（能改回来）。
  const broken = '[{oops'
  const events = []
  const props = reactive({ path: 'wizard/install', origin: broken, original: broken })
  const { api } = useEditor(
    props,
    (_event, value) => events.push(value),
    () => ({ steps: [] }),
    () => '[]\n',
  )
  check('解析失败时不算脏', api.isDirty(), false)
}

// ---- 5. 切视图时该交哪份文本（utils/handoff.ts）----
{
  const disk = '{"defaults":{"run-as":"package"}}\n'
  const broken = '[{'
  const emptyModel = '[]\n'

  // 表单模型不可信（解析失败降级成空模型，saveBlocked 非空）：必须交回表单**被加载时**
  // 的那份文本，也就是用户写坏前的那份。交空模型 = 用户刚写的文本被顶掉，而且空配置
  // 往往还「合法」，一保存就把文件清空。
  check(
    '模型不可信时交回加载时的文本（不是空模型）',
    handoffText({
      from: 'form',
      to: 'source',
      formDirty: true,
      formBlocked: '格式不对',
      formText: emptyModel,
      editorText: broken,
      diskText: disk,
    }),
    broken,
  )
  // 表单干净时交磁盘原文：回写会归一格式，拿它当初值会让没编辑过的文件一按「源码」
  // 就显示「未保存」。
  check(
    '表单干净时交磁盘原文',
    handoffText({
      from: 'form',
      to: 'source',
      formDirty: false,
      formBlocked: '',
      formText: 'NORMALIZED\n',
      editorText: disk,
      diskText: disk,
    }),
    disk,
  )
  // 表单脏且可信：交回写结果，用户刚才的编辑不能丢。
  check(
    '表单脏且可信时交回写结果',
    handoffText({
      from: 'form',
      to: 'source',
      formDirty: true,
      formBlocked: '',
      formText: 'EDITED\n',
      editorText: disk,
      diskText: disk,
    }),
    'EDITED\n',
  )
  // 源码 → 表单：交源码里的实时内容。
  check(
    '源码 → 表单交源码内容',
    handoffText({
      from: 'source',
      to: 'form',
      formDirty: false,
      formBlocked: '',
      formText: '',
      editorText: 'LIVE SOURCE\n',
      diskText: disk,
    }),
    'LIVE SOURCE\n',
  )
}

// ---- cmd/main 的插入模板 ----
//
// 模板是**纯文本**，写错的后果都在界面上：`${...}` 少转义一个反斜杠会被当成插值、
// 在源码里被换成别的东西（甚至编译报错），模板贴错则会让应用中心里「启动」点了
// 没反应。这两种都不会有编译期或运行期报错，所以钉在这里。

const mainOptions = mainTemplateOptions(false)
const dockerOptions = mainTemplateOptions(true)
check('非 docker 的 main 给「有进程 / 无进程」两项', mainOptions.map((o) => o.key), ['process', 'no-process'])
check('非 docker 的 main 不给 docker 模板', mainOptions.some((o) => o.key === 'docker'), false)
check('docker 的 main 多一份 docker 模板', dockerOptions.map((o) => o.key), ['process', 'no-process', 'docker'])
check('有进程模板的文本', mainOptions[0]?.text, MAIN_SCRIPT_TEMPLATE)
check('无进程模板的文本', mainOptions[1]?.text, MAIN_NO_PROCESS_TEMPLATE)
check('三份模板不是同一份文本（贴错就是两处都不对）', new Set([
  MAIN_SCRIPT_TEMPLATE,
  MAIN_NO_PROCESS_TEMPLATE,
  MAIN_DOCKER_SCRIPT_TEMPLATE,
]).size, 3)

for (const [label, text] of [
  ['有进程', MAIN_SCRIPT_TEMPLATE],
  ['无进程', MAIN_NO_PROCESS_TEMPLATE],
  ['docker', MAIN_DOCKER_SCRIPT_TEMPLATE],
]) {
  check(`${label}模板以 bash 开头`, text.startsWith('#!/bin/bash\n'), true)
  check(`${label}模板以换行结尾（插入后文件末尾有换行）`, text.endsWith('\n'), true)
  check(`${label}模板有 start / stop / status 三个分支`, /case \$1 in[\s\S]*start\)[\s\S]*stop\)[\s\S]*status\)[\s\S]*esac/.test(text), true)
}

// 有进程：自己托管进程（pid 文件 + TERM/KILL），且不该提到 docker
check('有进程模板写 pid 文件', MAIN_SCRIPT_TEMPLATE.includes('${PID_FILE}'), true)
check('有进程模板会发 TERM / KILL', MAIN_SCRIPT_TEMPLATE.includes('kill -TERM') && MAIN_SCRIPT_TEMPLATE.includes('kill -KILL'), true)
check('有进程模板不掺 docker 的东西', /docker/i.test(MAIN_SCRIPT_TEMPLATE), false)

// 无进程：四个子命令都只是 exit 0 / exit 1 的占位，不写 pid、不碰进程
check('无进程模板不写 pid', /PID_FILE|app\.pid/.test(MAIN_NO_PROCESS_TEMPLATE), false)
check('无进程模板不杀进程', /kill /.test(MAIN_NO_PROCESS_TEMPLATE), false)
for (const branch of ['start', 'stop', 'status', 'log']) {
  check(`无进程模板有 ${branch} 分支`, new RegExp(`${branch}\\)`).test(MAIN_NO_PROCESS_TEMPLATE), true)
}

// docker：容器由应用中心管，脚本什么都不做，只 status 去问容器
check('docker 模板 start 分支什么都不做', MAIN_DOCKER_SCRIPT_TEMPLATE.includes('will be started by appcenter'), true)
check('docker 模板问的是容器状态', MAIN_DOCKER_SCRIPT_TEMPLATE.includes('docker inspect'), true)
check('docker 模板不自己写 pid 文件', /PID_FILE/.test(MAIN_DOCKER_SCRIPT_TEMPLATE), false)


// ---- i18n locale 文件的模板与「缺 en-US」判据 ----
//
// 模板是纯文本，写错的后果都在界面上：少一个 section / key，用到它的那个字段在
// fnOS 侧就取不到值（应用中心里是空白）；值必须在（空的）双引号里——官方语法就是
// `key="value"`，漏掉引号那一行会被当成不认识的行。判据算错则是「该提示的不提示」，
// 只在英文环境下才看得出来（而且打包会被后端拦下，报错却指不到根因）。

check('en-US 用 locale 模板', i18nTemplateFor('i18n/en-US'), I18N_LOCALE_TEMPLATE)
check('zh-CN 用同一份 locale 模板', i18nTemplateFor('i18n/zh-CN'), I18N_LOCALE_TEMPLATE)
check('其他语言不给模板', i18nTemplateFor('i18n/ja-JP'), '')
check('i18n 以外的文件不给模板', i18nTemplateFor('manifest'), '')

for (const section of ['[common]', '[entry]', '[install]', '[uninstall]']) {
  check(`模板有 ${section}`, I18N_LOCALE_TEMPLATE.includes(`${section}\n`), true)
}
for (const key of [
  'displayName',
  'desc',
  'changelog',
  'title',
  'stepTitle',
  'label',
  'helpText',
  'option1Label',
  'option2Label',
]) {
  check(`模板保留了键 ${key}`, new RegExp(`^${key}=""$`, 'm').test(I18N_LOCALE_TEMPLATE), true)
}
check('模板以换行结尾', I18N_LOCALE_TEMPLATE.endsWith('\n'), true)
// 值必须留成空的双引号：官方语法是 `key="value"`，这份骨架只留键、不留示例值
const valueLines = I18N_LOCALE_TEMPLATE.split('\n').filter(
  (line) => line !== '' && !line.startsWith('['),
)
check(
  '每一行都是 `键=""`（值都留空）',
  valueLines.every((line) => /^[A-Za-z][A-Za-z0-9]*=""$/.test(line)),
  true,
)
check('模板里没有残留的示例值', I18N_LOCALE_TEMPLATE.includes('My App'), false)
check('模板里没有 ${ 插值（会被模板字面量求值吃掉）', I18N_LOCALE_TEMPLATE.includes('${'), false)

// 没有 i18n 目录（undefined）与「目录存在但缺 en-US」是两回事，别混成一个 false
check('没有 i18n 目录时不报', localeMissingEnUS(undefined), false)
check('目录存在但是空的，报缺 en-US', localeMissingEnUS([]), true)
check('只有 zh-CN 时报缺', localeMissingEnUS([{ name: 'zh-CN', dir: false }]), true)
check('有 en-US 文件就不报', localeMissingEnUS([{ name: 'en-US', dir: false }]), false)
check('同名的目录不算 en-US 文件', localeMissingEnUS([{ name: 'en-US', dir: true }]), true)
check(
  'en-US 和其他语言并存时不报',
  localeMissingEnUS([
    { name: 'zh-CN', dir: false },
    { name: 'en-US', dir: false },
  ]),
  false,
)

console.log(failed ? `\n${failed} 项不通过` : '\n编辑器脏判据检查全部通过')
process.exit(failed ? 1 : 0)
