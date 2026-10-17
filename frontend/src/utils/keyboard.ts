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
