/**
 * 横向滚动的小工具（标签栏这类「一行放不下」的列表用）。
 *
 * 浏览器默认只把纵向滚轮用来滚页面，横向列表得按住 Shift 或者去够那根细滚动条；
 * 这里把 delta 直接用到 scrollLeft 上。内容没溢出、或者已经滚到头时不拦事件，
 * 交回给页面继续滚。
 */
export function wheelToHorizontal(event: WheelEvent): void {
  const el = event.currentTarget as HTMLElement | null
  if (!el) return

  // 触摸板的横向手势优先用 deltaX；普通滚轮只有 deltaY
  const raw = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY
  if (!raw) return
  const delta = event.deltaMode === 1 ? raw * 16 : raw

  const max = el.scrollWidth - el.clientWidth
  if (max <= 0) return

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
