/**
 * 键盘事件。
 *
 * 回车在这些地方等于「提交」：新建文件 / 目录、重命名、新建项目、入口 id —— 而它们
 * 都可能是一个中文名字，所以提交前都要让过输入法的组词回车（见 `onEnter`）。
 */

/**
 * 回车提交。
 *
 * 中文 / 日文输入法用回车选词时，浏览器照样会派发 Enter 的 `keydown` —— 不判
 * `isComposing` 的话，名字还没选完就把半截提交了（比如「测试」只打出「ce」）。
 *
 * **必须挂在 `keydown` 上**：组词提交之后那一下 `keyup` 里 `isComposing` 已经是
 * false，在 `keyup` 上判不到。
 *
 * 参数收 `Event` 而不是 `KeyboardEvent`：Element Plus 的 `el-input` 把 `keydown`
 * 的载荷声明成 `Event | KeyboardEvent`，收窄成 `KeyboardEvent` 绑不上去。只在
 * `@keydown.enter` 里调用，运行时一定是键盘事件。
 */
export function onEnter(event: Event, run: () => void): void {
  if ((event as KeyboardEvent).isComposing) return
  run()
}

/**
 * 快捷键要不要让给正在输入的控件。
 *
 * 文件管理器的 Ctrl+C / X / V / Z 是**文件操作**；焦点在输入框、文本域、下拉或
 * contenteditable（CodeMirror 的编辑区就是）里时必须让给浏览器处理，否则用户想复制
 * 一段文字，复制走的却是选中的文件。
 */
export function isTypingTarget(event: Event): boolean {
  const target = event.target as HTMLElement | null
  if (!target) return false
  const tag = target.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (target.isContentEditable) return true
  return Boolean(target.closest?.('[contenteditable="true"]'))
}

/**
 * 是否有一个可见的 Element Plus 遮罩（弹窗 / 抽屉 / 确认框）。
 *
 * 文件管理器的快捷键挂在 window 上，弹窗打开时焦点可能在弹窗按钮上；这时再按
 * Ctrl+C，复制的会是底下的文件。遮罩关掉之后元素可能还留在 DOM 里（`display:none`），
 * 所以要同时看计算样式和实际尺寸，不能只查选择器。
 */
export function hasOpenOverlay(): boolean {
  for (const el of document.querySelectorAll<HTMLElement>('.el-overlay')) {
    if (el.getAttribute('aria-hidden') === 'true') continue
    const style = window.getComputedStyle(el)
    if (style.display === 'none' || style.visibility === 'hidden') continue
    if (el.getBoundingClientRect().width > 0) return true
  }
  return false
}
