/**
 * 从 `app/<入口目录>/config` 里读出口的 id。
 *
 * 入口 id 是 `.url` 这个对象的**键**（值里没有 id 字段），所以只能取键名。manifest 的
 * 「默认入口」要填的就是这里的一个 id，用户在 manifest 编辑器里点开那个字段时现读一次
 * 这份 config，拿到最新的入口列表当候选——不然「刚在入口配置里加了一个入口，回头填默认
 * 入口时下拉里没有它」。
 *
 * 解析不了、或者 `.url` 不是对象时返回空数组：这只是「没有候选可挑」，不该拦着用户手输，
 * 也不该弹错误（那份 config 的问题会由它自己的编辑器报出来）。
 */
import { parseJSON } from './json'

export function entryIds(text: string): string[] {
  const parsed = parseJSON<Record<string, unknown>>(text, {})
  const raw = parsed.value
  if (parsed.error || !raw || typeof raw !== 'object' || Array.isArray(raw)) return []
  const urls = raw['.url']
  if (!urls || typeof urls !== 'object' || Array.isArray(urls)) return []
  return Object.keys(urls as Record<string, unknown>)
}
