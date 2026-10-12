/**
 * 横向滚动的小工具（标签栏这类「一行放不下」的列表用）。
 *
 * 浏览器默认只把纵向滚轮用来滚页面，横向列表得按住 Shift 或者去够那根细滚动条；
 * 这里把 delta 直接用到 scrollLeft 上。内容没溢出、已经滚到头、或者（按下面的选项）
 * 纵向还有得滚时不拦事件，交回给页面继续滚。
 */

export interface WheelToHorizontalOptions {
  /**
   * 只有容器纵向**滚不动**时才接管滚轮。
   *
   * 给「一个容器同时管纵向和横向」的地方用（首页项目区：网格视图纵向滚、
   * 列表视图横向滚）。不判这一条的话，两个方向都溢出时纵向就再也滚不动了
   * ——网格在很窄的容器里（卡片最小 17rem）正好会横向溢出。
   */
  onlyWhenNoVerticalScroll?: boolean
}

export function wheelToHorizontal(event: WheelEvent, options: WheelToHorizontalOptions = {}): void {
  const el = event.currentTarget as HTMLElement | null
  if (!el) return

  // 触摸板的横向手势优先用 deltaX；普通滚轮只有 deltaY
  const raw = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY
  if (!raw) return
  const delta = event.deltaMode === 1 ? raw * 16 : raw

  const max = el.scrollWidth - el.clientWidth
  if (max <= 0) return
  // 容差 1px：scrollHeight 会向上取整，正好装满时也可能多出 1
  if (options.onlyWhenNoVerticalScroll && el.scrollHeight - el.clientHeight > 1) return

  const next = Math.min(max, Math.max(0, el.scrollLeft + delta))
  if (next === el.scrollLeft) return

  event.preventDefault()
  el.scrollLeft = next
}

/** 把容器里当前激活的那一项滚进可视区（切标签时用）。 */
export function revealActive(
  container: HTMLElement | null | undefined,
  selector: string,
): void {
  container?.querySelector(selector)?.scrollIntoView({ inline: 'nearest', block: 'nearest' })
}
