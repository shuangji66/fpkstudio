/**
 * 「表单 / 源码」两个视图之间切换时，该把哪份文本交给新挂载的编辑器。
 *
 * 单独成模块是因为这段规则踩过一次大坑：表单的模型可能是**解析失败后的降级模型**
 * （`useEditor` 的 load 返回空模型 + error）。那时 `isDirty()` 是 true——基线是磁盘
 * 文本走一遍解析 + 回写（非空），而回写出来的是一份空配置——于是「表单 → 源码」交出去
 * 的就是那份空配置：用户刚在源码里写的文本被顶掉（组件按 `path:mode` 做 key，重建之后
 * 连撤销都回不来），而 `[]` / `{"defaults":{"run-as":"package"}}` 这类空配置往往还是
 * **合法**的，源码视图的 `saveBlocked` 拦不住，一保存就把用户的文件清空。
 *
 * 判据：模型不可信（编辑器自己报了 `saveBlocked`）时，交回**表单被加载时的那份文本**。
 * 它就是用户写坏之前的那一份：既保住了文本，又能在源码视图里接着改。
 *
 * 放在 `utils/` 里是为了能进 `check:editor` 单独验（组件里的那半只能手点）。
 */

export type EditorViewMode = 'form' | 'source'

export interface HandoffInput {
  from: EditorViewMode
  to: EditorViewMode
  /** 表单编辑器的脏标记（`isDirty`）。 */
  formDirty: boolean
  /** 表单编辑器给出的「现在不能保存」的原因；非空表示模型不可信。 */
  formBlocked: string
  /** 表单编辑器回写出来的文本（`getContent`）。 */
  formText: string
  /** 当前编辑器挂载时拿到的那份文本（表单：它在 host 里的 `loadText`；源码：`getText()`）。 */
  editorText: string
  /** 磁盘上的原文。 */
  diskText: string
}

/** 返回新挂载的编辑器应当拿到的文本。 */
export function handoffText(input: HandoffInput): string {
  if (input.from === 'form' && input.to === 'source') {
    // 模型不可信：绝不能把降级出来的空模型当成用户的改动交出去（见文件头）。
    if (input.formBlocked) return input.editorText
    // 干净时交磁盘原文：回写会归一格式（缩进、键序、补默认键），拿回写结果当初值会让
    // 一份没编辑过的脚手架文件一按「源码」就显示「未保存」。
    if (!input.formDirty) return input.diskText
    return input.formText || input.editorText
  }
  if (input.from === 'source' && input.to === 'form') {
    // 源码 → 表单：把源码里的实时内容作为解析输入。
    return input.editorText
  }
  // 同一个视图之间不切换（正常走不到）。
  return input.editorText
}
