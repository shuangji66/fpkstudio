/**
 * 校验表单编辑器的「未改动」判据（src/components/editor/useEditor.ts）。
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

console.log(failed ? `\n${failed} 项不通过` : '\n编辑器脏判据检查全部通过')
process.exit(failed ? 1 : 0)
