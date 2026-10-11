/**
 * 语义路径（`trim.file.convertPath`）结果的整理。
 *
 * 单独成模块是因为它踩过一次：拿返回的 `path` 精确匹配请求路径，只要系统回
 * `/vol1/1000/fpk/`（多个尾斜杠）就对不上，界面上「什么也不显示」，还查不出原因。
 * 这个模块只管把「请求了哪几条路径」和「系统回了哪些条目」对上号，纯函数、可单测
 * （见 scripts/check-semantic.mjs）。
 */
import type { SemanticPath } from '@/api/types'

/** 归一化：去掉尾部的 `/`（`/a/b` 与 `/a/b/` 是同一个目录）。 */
function normalize(path: string): string {
  const trimmed = path.replace(/\/+$/, '')
  return trimmed === '' ? '/' : trimmed
}

/**
 * 把结果按「请求过的路径」重新组织。
 *
 * 两级匹配：先按归一化后的路径找；找不到而条目数与请求数一致时按位置取（文档里
 * `result` 与请求的 `path` 一一对应、顺序一致）。系统回空串或不回某条，就当那条
 * 没有语义路径——界面退回显示原始路径，不显示半截东西。
 */
export function indexSemantics(
  requested: string[],
  result: SemanticPath[],
): Record<string, string> {
  const out: Record<string, string> = {}
  if (!requested.length || !result.length) return out

  const byPath = new Map<string, string>()
  for (const item of result) {
    if (item && typeof item.path === 'string' && item.semanticPath) {
      byPath.set(normalize(item.path), item.semanticPath)
    }
  }

  requested.forEach((path, index) => {
    const hit = byPath.get(normalize(path)) ?? result[index]?.semanticPath
    if (hit) out[path] = hit
  })
  return out
}
