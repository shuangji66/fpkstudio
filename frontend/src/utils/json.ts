/**
 * JSON 的解析与回写约定。
 *
 * fpk 的配置文件里，同一份内容可能有多种写法（键序、缩进），而这里的原则是
 * 「只改用户真正编辑过的东西」，所以序列化统一按固定键序输出，避免每次保存
 * 都产生无意义的 diff。
 */

/** 安全解析 JSON，失败时返回兜底值并把错误交给调用方展示。 */
export function parseJSON<T>(text: string, fallback: T): { value: T; error: string } {
  const trimmed = text.trim()
  if (!trimmed) return { value: fallback, error: '' }
  try {
    return { value: JSON.parse(trimmed) as T, error: '' }
  } catch (e) {
    return { value: fallback, error: e instanceof Error ? e.message : String(e) }
  }
}

/**
 * 按固定键顺序序列化对象，结尾补一个换行。
 *
 * `first` 里的键排在最前面（按给的顺序），其余仍按字母序。纯字母序有时把「主语」
 * 排到后面去——向导里 `items` 会排到 `stepTitle` 前面，读源码时得先扫完一整屏表单项
 * 才知道这一步叫什么。
 */
export function stringifyOrdered(value: unknown, first: string[] = []): string {
  return `${JSON.stringify(sortKeys(value, first), null, 2)}\n`
}

function sortKeys(value: unknown, first: string[]): unknown {
  if (Array.isArray(value)) return value.map((item) => sortKeys(item, first))
  if (value && typeof value === 'object') {
    const source = value as Record<string, unknown>
    const out: Record<string, unknown> = {}
    for (const key of orderKeys(Object.keys(source), first)) {
      out[key] = sortKeys(source[key], first)
    }
    return out
  }
  return value
}

/** 先按 `first` 里的位置排，再按字母序；`first` 里没有的键位置记为 -1（排后面）。 */
function orderKeys(keys: string[], first: string[]): string[] {
  if (!first.length) return [...keys].sort()
  const rank = (key: string) => {
    const index = first.indexOf(key)
    return index === -1 ? first.length : index
  }
  return [...keys].sort((a, b) => {
    const diff = rank(a) - rank(b)
    if (diff !== 0) return diff
    return a < b ? -1 : a > b ? 1 : 0
  })
}
