/**
 * 首页卡片的置顶：排序与增删的纯逻辑。
 *
 * 置顶只是**界面偏好**，不进后端、不进 `state.json`：它记在浏览器的 localStorage
 * （`fpkstudio.pinnedProjects`），换台机器打开同一个项目根就是没置顶的状态。所以这里
 * 只算「这批项目怎么排」和「点一下之后置顶表变成什么」，真正的读写在 stores/projects.ts。
 *
 * 「先置顶的排前面」：数组顺序就是点「置顶」的先后，取消再置顶会重新排到后面——最近
 * 置顶的离手最近，比「按名字排序」直观。
 */

/** 置顶一个项目：已经在里面就原样返回（幂等，不做重复项）。 */
export function addPin(pins: readonly string[], name: string): string[] {
  if (pins.includes(name)) return [...pins]
  return [...pins, name]
}

/** 取消置顶；不在里面就原样返回。 */
export function removePin(pins: readonly string[], name: string): string[] {
  return pins.filter((pin) => pin !== name)
}

/** 点一下「置顶 / 取消置顶」。 */
export function togglePin(pins: readonly string[], name: string): string[] {
  return pins.includes(name) ? removePin(pins, name) : addPin(pins, name)
}

/** 项目改名之后置顶要跟着走，否则那条置顶会变成一个不存在的名字。 */
export function renamePin(pins: readonly string[], from: string, to: string): string[] {
  if (!pins.includes(from)) return [...pins]
  return pins.map((pin) => (pin === from ? to : pin))
}

/**
 * 把置顶的项目排到前面（置顶内部按置顶顺序，其余保持后端给的顺序）。
 *
 * 用 `sort` 而不是先 filter 再拼接：现代 JS 的排序是稳定的，没置顶的那些相对顺序
 * 原样保留——「按最后修改时间排」这种后端顺序不该因为置顶功能被打乱。
 * 置顶表里有、但这次列表里没有的名字（被删掉的项目、被搜索关键字筛掉的）不影响排序。
 */
export function sortPinned<T extends { name: string }>(items: readonly T[], pins: readonly string[]): T[] {
  if (!pins.length) return [...items]
  const rank = new Map(pins.map((name, index) => [name, index]))
  const last = Number.MAX_SAFE_INTEGER
  return [...items].sort((a, b) => (rank.get(a.name) ?? last) - (rank.get(b.name) ?? last))
}
