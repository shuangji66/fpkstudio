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

/** 按固定键顺序序列化对象，结尾补一个换行。 */
export function stringifyOrdered(value: unknown): string {
  return `${JSON.stringify(sortKeys(value), null, 2)}\n`
}

function sortKeys(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortKeys)
  if (value && typeof value === 'object') {
    const source = value as Record<string, unknown>
    const out: Record<string, unknown> = {}
    for (const key of Object.keys(source).sort()) out[key] = sortKeys(source[key])
    return out
  }
  return value
}
