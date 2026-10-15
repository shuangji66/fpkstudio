import { parseJSON } from './json'
import { Manifest } from './manifest'
import { parseWizard } from './wizard'

/**
 * 源码视图保存前的格式校验。
 *
 * 表单视图的校验在各编辑器的 `saveBlocked` 里（它们手上有解析后的模型可看），但
 * **源码视图能绕过去直接把文本写进文件**：一份写坏的 JSON 会让那个文件在 fnOS 里
 * 直接失效，而用户往往是照着别处抄的时候漏了个括号。所以这里按路径再拦一道。
 *
 * 只认「有确定格式」的文件：manifest、几份 JSON 配置、向导、i18n 的 locale 文件
 * （官方是 INI 语法，不是 JSON）。生命周期脚本（cmd/*）和普通文本没有格式可言，
 * 不校验——脚本语法不该由这里说了算。
 *
 * 返回的是「用哪个 i18n 键 + 什么参数」，不是拼好的句子：这里要能单独跑检查
 * （`check:format`），不该把 vue-i18n 拖进来。
 */
export interface FormatIssue {
  /** 给用户看的原因用的 i18n 键，参数在 params 里。 */
  key: string
  params?: Record<string, unknown>
}

/** 校验一段文本在某个路径上是不是合法；没问题返回 null。 */
export function checkFormat(path: string, text: string): FormatIssue | null {
  const rel = path.replace(/^\.?\//, '')

  if (rel === 'manifest') return checkManifest(text)

  if (/^wizard\//.test(rel)) {
    const { error } = parseWizard(text)
    return error ? { key: 'editor.formatWizard', params: { detail: error } } : null
  }

  if (isJSONConfig(rel)) {
    const { error } = parseJSON(text, null)
    return error ? { key: 'editor.formatJson', params: { detail: error } } : null
  }

  if (rel.startsWith('i18n/')) return checkI18nLocale(text)

  return null
}

/** 这些路径上的文件按 JSON 存。 */
function isJSONConfig(rel: string): boolean {
  return (
    rel === 'config/privilege' ||
    rel === 'config/resource' ||
    /^app\/[^/]+\/config$/.test(rel)
  )
}

/**
 * i18n 的 locale 文件：官方语法是 INI（`[section]` + `key="值"`），**不是 JSON**。
 *
 * 这里以前把它和几份 JSON 配置归成一类，判据完全对不上——一份合法的 locale 文件
 * 在源码视图里根本存不进去（而表单视图没有这个文件的编辑器，只有源码这一条路）。
 *
 * 挑的仍是「会静默失效」的错：非空、非注释，又不是 `[section]` 或 `key=...` 的行
 * ——fnOS 直接把那一行忽略掉，用户以为配上了其实没有；以及引号只开不闭的值——
 * 解析器会把引号后面的内容一起当成值。值的写法本身不挑（`key=值` 不带引号也放行）。
 */
function checkI18nLocale(text: string): FormatIssue | null {
  // 头部的 BOM 会让第一行（往往就是 `[common]`）判不出来。
  const lines = text.replace(/^\uFEFF/, '').split(/\r?\n/)
  for (const raw of lines) {
    const line = raw.trim()
    if (line === '' || line.startsWith('#') || line.startsWith(';')) continue
    if (/^\[[^[\]]+\]$/.test(line)) continue
    const kv = /^([A-Za-z_][\w.-]*)\s*=\s*(.*)$/.exec(line)
    if (!kv) return { key: 'editor.formatI18n', params: { line } }
    const value = kv[2]!.trim()
    if (value.startsWith('"') && !/^".*"$/.test(value)) {
      return { key: 'editor.formatI18nQuote', params: { key: kv[1] } }
    }
  }
  return null
}

/**
 * manifest 的校验。
 *
 * 它没有「解析失败」这一说（`Manifest.parse` 把认不出来的行原样留着），所以这里
 * 挑的是**真正会静默失效**的那种错：非空、非注释、又不是 `key = value` 的行——
 * fnpack 会直接忽略它，用户以为配上了，其实没有。
 *
 * 判据建立在解析结果上而不是逐行正则：`desc = """多行"""` 里的续行会被解析器吃进
 * 值里，逐行看会把它们全判成错误。
 */
function checkManifest(text: string): FormatIssue | null {
  const parsed = Manifest.parse(text)
  // 多行值没闭合：解析器一路读到了文件结尾，它后面的键都进了这个值里，保存一次就会
  // 丢键（见 Manifest.unterminated）。源码视图这一侧靠这条拦。
  if (parsed.unterminated.length) {
    return {
      key: 'editor.formatManifestMultiline',
      params: { keys: parsed.unterminated.join(', ') },
    }
  }
  for (const entry of parsed.entries) {
    if (entry.kind !== 'raw') continue
    const line = entry.raw.trim()
    if (line === '' || line.startsWith('#')) continue
    return { key: 'editor.formatManifest', params: { line } }
  }
  return null
}
