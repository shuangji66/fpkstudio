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
