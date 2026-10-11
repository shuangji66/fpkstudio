/**
 * manifest 的解析与回写。
 *
 * 比较讲究的两点，都是 fnpackup 做不到的：
 *
 *  1. 官方 fnpack 生成的模板是**对齐过的**（`appname               = demo`），
 *     而且用户可能写注释、写空行。这里把每一行原样记住，回写时保持顺序、
 *     注释和分隔符风格，只改动真正被编辑过的键——避免「打开一次 manifest
 *     保存后整个文件被重排」这种事情。
 *  2. `desc` 常常是 `"""多行内容"""`，不能被按行切开（fnpackup 的实现会把
 *     第二行当成一个新的键）。
 */

export interface ManifestEntry {
  kind: 'kv' | 'raw'
  /** kind === 'raw' 时原样输出 */
  raw: string
  key: string
  value: string
  /** `=` 两侧的空白风格，例如 `=` 或 ` = ` */
  sep: string
}

function splitLines(text: string): string[] {
  return text.replace(/\r\n?/g, '\n').split('\n')
}

export class Manifest {
  entries: ManifestEntry[] = []
  private index = new Map<string, number>()

  static parse(text: string): Manifest {
    const m = new Manifest()
    const lines = splitLines(text)

    for (let i = 0; i < lines.length; i += 1) {
      const line = lines[i]!
      const trimmed = line.trim()

      if (trimmed === '' || trimmed.startsWith('#')) {
        m.push({ kind: 'raw', raw: line, key: '', value: '', sep: '' })
        continue
      }

      const eq = trimmed.indexOf('=')
      if (eq <= 0) {
        m.push({ kind: 'raw', raw: line, key: '', value: '', sep: '' })
        continue
      }

      const left = trimmed.slice(0, eq)
      const right = trimmed.slice(eq + 1)
      const key = left.trim()
      const sep = `${left === key ? '' : ' '}=${right === right.trimStart() ? '' : ' '}`

      if (!key) {
        m.push({ kind: 'raw', raw: line, key: '', value: '', sep: '' })
        continue
      }

      // 三引号多行值
      if (right.trimStart().startsWith('"""')) {
        const stripped = right.trimStart().slice(3)
        if (stripped.endsWith('"""') && stripped.length >= 3) {
          m.push({ kind: 'kv', raw: line, key, value: stripped.slice(0, -3), sep })
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
        m.push({
          kind: 'kv',
          raw: line,
          key,
          value: closed ? buffer.join('\n') : buffer.join('\n'),
          sep,
        })
        continue
      }

      m.push({ kind: 'kv', raw: line, key, value: unquote(right.trim()), sep: sep || '=' })
      if (!sep) m.entries[m.entries.length - 1]!.sep = '='
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
      if (!existing.sep) existing.sep = '='
      return
    }
    // 新键插到末尾空行之前，保持文件结尾整洁
    let insertAt = this.entries.length
    while (insertAt > 0 && this.entries[insertAt - 1]!.kind === 'raw' &&
      this.entries[insertAt - 1]!.raw.trim() === '') {
      insertAt -= 1
    }
    const entry: ManifestEntry = { kind: 'kv', raw: '', key, value, sep: '=' }
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

  keys(): string[] {
    return this.entries.filter((e) => e.kind === 'kv').map((e) => e.key)
  }

  toText(): string {
    return this.entries
      .map((entry) => {
        if (entry.kind === 'raw') return entry.raw
        return `${entry.key}${entry.sep}${renderValue(entry.value)}`
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
