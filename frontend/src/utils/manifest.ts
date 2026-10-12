/**
 * manifest 的解析与回写。
 *
 * 比较讲究的两点，都是 fnpackup 做不到的：
 *
 *  1. 官方 fnpack 生成的模板是**对齐过的**（`appname               = demo`），
 *     而且用户可能写注释、写空行、缩进。这里把每一行原样记住（`=` 左边的全部字符，
 *     含行首缩进与对齐用的空格），回写时保持顺序、注释、缩进和分隔符风格，只改动
 *     真正被编辑过的键——避免「打开一次 manifest 保存后整个文件被重排」这种事。
 *     会被规范化的是**值的写法**：`"1.0.0"` 的引号、值后面的行尾空格不会保留
 *     （值本身不变），多行值统一写成 `"""..."""`。用户改的是值，不是引号。
 *  2. `desc` 常常是 `"""多行内容"""`，不能被按行切开（fnpackup 的实现会把
 *     第二行当成一个新的键）。
 */

export interface ManifestEntry {
  kind: 'kv' | 'raw'
  /** kind === 'raw' 时原样输出 */
  raw: string
  key: string
  value: string
  /** `=` 左边原样保留的文本（含行首缩进与对齐用的空格）；新建的键是 `key + ' '`。 */
  left: string
  /** `=` 右边原样的空格，例如对齐模板里 `= value` 的那一个空格。 */
  pre: string
}

function splitLines(text: string): string[] {
  return text.replace(/\r\n?/g, '\n').split('\n')
}

export class Manifest {
  entries: ManifestEntry[] = []
  private index = new Map<string, number>()

  /**
   * 多行值（`"""`）一直没闭合的键。
   *
   * 未闭合时解析器只能一路读到文件结尾——不能因为一个没写完的引号就把整份文件判成
   * 「解析失败」，它明明还能显示、还能改。代价是**后面的键全被吞进这个值里**：`version`
   * / `platform` 会从模型里消失，保存时被埋进 `desc` 或另起一行空值。所以这里把「谁
   * 没闭合」记下来，交给 `ManifestEditor.saveBlocked` 和 `utils/validate.ts` 拦保存。
   */
  unterminated: string[] = []

  static parse(text: string): Manifest {
    const m = new Manifest()
    const lines = splitLines(text)

    for (let i = 0; i < lines.length; i += 1) {
      const line = lines[i]!
      const trimmed = line.trim()

      if (trimmed === '' || trimmed.startsWith('#')) {
        m.push({ kind: 'raw', raw: line, key: '', value: '', left: '', pre: '' })
        continue
      }

      const eq = trimmed.indexOf('=')
      if (eq <= 0) {
        m.push({ kind: 'raw', raw: line, key: '', value: '', left: '', pre: '' })
        continue
      }

      const left = trimmed.slice(0, eq)
      const right = trimmed.slice(eq + 1)
      const key = left.trim()
      // 原样记住 "=" 左边的全部字符：行首缩进 + 对齐用的空格。fnpack 的模板是对齐过的，
      // 保存时不能压成 `key = value`，否则「只改一个字段」会把整份文件重排
      // （模块注释的承诺）。
      const indent = line.slice(0, line.length - line.trimStart().length)
      const pad = indent + left
      const pre = right.slice(0, right.length - right.trimStart().length) || ' '

      if (!key) {
        m.push({ kind: 'raw', raw: line, key: '', value: '', left: '', pre: '' })
        continue
      }

      // 三引号多行值
      if (right.trimStart().startsWith('"""')) {
        const stripped = right.trimStart().slice(3)
        if (stripped.endsWith('"""') && stripped.length >= 3) {
          m.push({ kind: 'kv', raw: line, key, value: stripped.slice(0, -3), left: pad, pre })
          continue
        }
        const buffer = [stripped]
        let closed = false
        while (i + 1 < lines.length) {
          i += 1
          const next = lines[i]!
          const at = next.indexOf('"""')
          if (at >= 0) {
            buffer.push(next.slice(0, at))
            closed = true
            break
          }
          buffer.push(next)
        }
        if (!closed) m.unterminated.push(key)
        m.push({
          kind: 'kv',
          raw: line,
          key,
          value: buffer.join('\n'),
          left: pad,
          pre,
        })
        continue
      }

      m.push({ kind: 'kv', raw: line, key, value: unquote(right.trim()), left: pad, pre })
    }

    return m
  }

  private push(entry: ManifestEntry) {
    if (entry.kind === 'kv') this.index.set(entry.key, this.entries.length)
    this.entries.push(entry)
  }

  get(key: string): string {
    const at = this.index.get(key)
    if (at === undefined) return ''
    return this.entries[at]!.value
  }

  has(key: string): boolean {
    return this.index.has(key)
  }

  set(key: string, value: string) {
    const at = this.index.get(key)
    if (at !== undefined) {
      const existing = this.entries[at]!
      existing.value = value
      return
    }
    // 新键插到末尾空行之前，保持文件结尾整洁
    let insertAt = this.entries.length
    while (insertAt > 0 && this.entries[insertAt - 1]!.kind === 'raw' &&
      this.entries[insertAt - 1]!.raw.trim() === '') {
      insertAt -= 1
    }
    // 新键写成 `key = value`（与模板的写法一致）
    const entry: ManifestEntry = { kind: 'kv', raw: '', key, value, left: `${key} `, pre: ' ' }
    this.entries.splice(insertAt, 0, entry)
    this.reindex()
  }

  remove(key: string) {
    const at = this.index.get(key)
    if (at === undefined) return
    this.entries.splice(at, 1)
    this.reindex()
  }

  /** 删除值为空的键（表单编辑的语义：清空 = 不声明）。 */
  compact(keep: string[] = []) {
    const keepSet = new Set(keep)
    this.entries = this.entries.filter(
      (e) => e.kind === 'raw' || keepSet.has(e.key) || e.value.trim() !== '',
    )
    this.reindex()
  }

  private reindex() {
    this.index.clear()
    this.entries.forEach((e, i) => {
      if (e.kind === 'kv') this.index.set(e.key, i)
    })
  }

  toText(): string {
    return this.entries
      .map((entry) => {
        if (entry.kind === 'raw') return entry.raw
        return `${entry.left || entry.key}=${entry.pre}${renderValue(entry.value)}`
      })
      .join('\n')
  }
}

function unquote(value: string): string {
  if (value.length >= 2) {
    const first = value[0]
    const last = value[value.length - 1]
    if (first === last && (first === '"' || first === "'" || first === '`')) {
      return value.slice(1, -1)
    }
  }
  return value
}

function renderValue(value: string): string {
  if (value.includes('\n') || value.includes('"""')) {
    return `"""${value}"""`
  }
  return value
}
