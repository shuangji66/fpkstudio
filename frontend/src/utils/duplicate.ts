import { parseJSON, stringifyOrdered } from './json'
import { Manifest } from './manifest'

/**
 * 「创建副本」写盘前对两份文件的改写：把副本里的应用标识换成副本项目名。
 *
 * 只碰这两份，其他文件在复制时原样落地（谁调用见 `components/home/ProjectActions.vue`）：
 *  - `manifest` 的 `appname`，以及 `desktop_applaunchname`（默认入口 id）的前缀；
 *  - `app/ui/config` 里 `.url` 的每个入口 id 前缀。
 *
 * 放在 `utils/` 是为了能进 `check:duplicate` 单独验：这里算错的表现是「副本装上去
 * 之后桌面入口点不开」或者「副本和原项目抢同一个应用标识」，而两种都不会报错。
 */

/**
 * 把入口 id 的前缀换成新的应用标识：`myapp.main` → `copy.main`。
 *
 * 入口 id 的写法是 `<appname>.<入口名>`，所以换的是**第一个点之前**那一段。
 * 没有点、或者点在最前面的 id 原样返回——那不是「前缀 + 入口名」的写法，猜着改
 * 只会把一个本来能用的 id 改坏（宁可留着让用户自己在界面上改）。
 */
export function replaceEntryPrefix(id: string, appname: string): string {
  const at = id.indexOf('.')
  if (at <= 0) return id
  return `${appname}${id.slice(at)}`
}

/** `app/ui/config` 为什么没改：`missing` = 原项目没有这个文件，`invalid` = 解析不了。 */
export type UiConfigSkip = '' | 'missing' | 'invalid'

export interface DuplicateRewrite {
  /** 改写后的 manifest 原文（没动的行、注释、对齐都原样保留）。 */
  manifest: string
  /** 改写后的 app/ui/config 原文；**null = 这份文件不写回**（见 `uiConfigSkipped`）。 */
  uiConfig: string | null
  uiConfigSkipped: UiConfigSkip
}

/**
 * 把两份文本改成副本该有的样子。
 *
 * `uiConfigText` 给 null（原项目不带桌面入口）时不动那份文件；**解析不了时也不动**：
 * 那说明这份 config 已经坏了（或者格式不是我们认识的那套），拿解析结果回写等于用一份
 * 残缺配置盖掉用户原来的内容——和编辑器里「解析失败就拦保存」是同一条理由。
 */
export function rewriteForDuplicate(
  appname: string,
  manifestText: string,
  uiConfigText: string | null,
): DuplicateRewrite {
  const manifest = Manifest.parse(manifestText)
  manifest.set('appname', appname)
  const launch = manifest.get('desktop_applaunchname')
  if (launch) manifest.set('desktop_applaunchname', replaceEntryPrefix(launch, appname))

  if (uiConfigText === null) {
    return { manifest: manifest.toText(), uiConfig: null, uiConfigSkipped: 'missing' }
  }

  const parsed = parseJSON<Record<string, unknown>>(uiConfigText, {})
  const raw = parsed.value
  if (parsed.error || !raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return { manifest: manifest.toText(), uiConfig: null, uiConfigSkipped: 'invalid' }
  }

  const urls = raw['.url']
  if (!urls || typeof urls !== 'object' || Array.isArray(urls)) {
    // 没有 `.url` 就没有入口要改，这份文件原样留着（别顺手把它重新序列化一遍）
    return { manifest: manifest.toText(), uiConfig: null, uiConfigSkipped: '' }
  }

  const next: Record<string, unknown> = {}
  for (const [id, entry] of Object.entries(urls as Record<string, unknown>)) {
    next[replaceEntryPrefix(id, appname)] = entry
  }
  // 键序交给 stringifyOrdered（按字母序），和 config 编辑器保存时的写法一致
  return {
    manifest: manifest.toText(),
    uiConfig: stringifyOrdered({ ...raw, '.url': next }),
    uiConfigSkipped: '',
  }
}
