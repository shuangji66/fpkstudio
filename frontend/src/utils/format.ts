/** 展示用的小工具。 */

export function baseName(path: string): string {
  const at = path.lastIndexOf('/')
  return at < 0 ? path : path.slice(at + 1)
}

export function parentPath(path: string): string {
  const at = path.lastIndexOf('/')
  return at < 0 ? '' : path.slice(0, at)
}

export function joinPath(...parts: string[]): string {
  return parts
    .filter((p) => p !== '')
    .join('/')
    .replace(/\/{2,}/g, '/')
}

/**
 * 在同一个目录里挑一个不重名的名字：`a.txt` → `a - 副本.txt` → `a - 副本2.txt`…
 *
 * 粘贴时用它避开已有文件——文件管理器里的「副本」就是这么来的。宁可选个笨名字，
 * 也不要覆盖用户已经有的东西。带扩展名的沿用扩展名，目录名（没有扩展名）直接加后缀。
 */
export function uniqueName(existing: { name: string }[], name: string, suffix: string): string {
  const taken = new Set(existing.map((entry) => entry.name))
  if (!taken.has(name)) return name

  const dot = name.lastIndexOf('.')
  const base = dot > 0 ? name.slice(0, dot) : name
  const ext = dot > 0 ? name.slice(dot) : ''
  let candidate = `${base} - ${suffix}${ext}`
  for (let i = 2; taken.has(candidate); i += 1) candidate = `${base} - ${suffix}${i}${ext}`
  return candidate
}

export function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(1)} ${units[unit]}`
}

/** 判断某个路径是否命中编辑器的匹配规则。 */
export function matchesPath(pattern: RegExp, path: string): boolean {
  return pattern.test(path.replace(/^\.?\//, ''))
}

/**
 * 项目在界面上的名字：一律用应用唯一标识 appname。
 *
 * 不用 display_name：fnpack 模板里它默认就是占位符 `display-name`，所有新项目
 * 都叫同一个名字，标签页根本分不清谁是谁。manifest 里没写 appname 时退回目录名。
 */
export function projectLabel(info: { name: string; appname?: string }): string {
  return info.appname?.trim() || info.name
}
