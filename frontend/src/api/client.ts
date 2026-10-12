import { apiURL } from '@/runtime'
import { i18n } from '@/i18n'

/** 后端返回 {error: "..."} 时抛出。 */
export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

type QueryValue = string | number | boolean | undefined | null

interface RequestOptions {
  method?: string
  json?: unknown
  query?: Record<string, QueryValue>
  form?: FormData
  signal?: AbortSignal
}

function buildQuery(query?: Record<string, QueryValue>): string {
  if (!query) return ''
  const parts: string[] = []
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null) continue
    parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`)
  }
  return parts.length ? `?${parts.join('&')}` : ''
}

/** 把响应体裁成一小段，用于把「不是 JSON」的回包原样暴露出来。 */
function snippet(text: string, max = 200): string {
  const one = text.replace(/\s+/g, ' ').trim()
  return one.length > max ? `${one.slice(0, max)}…` : one
}

async function extractError(res: Response): Promise<never> {
  let detail = `${res.status} ${res.statusText}`
  try {
    const text = await res.text()
    if (text) {
      try {
        const parsed = JSON.parse(text) as { error?: string }
        if (parsed.error) detail = parsed.error
        else detail = text
      } catch {
        // 不是后端的 {error:…}：多半是中间的网关 / 反代插了一脚（登录页、
        // 令牌提示之类）。原样带出来，不然只会看到一个 JSON 解析错误。
        // 这几句会经 e.message 原样显示给用户，所以走 i18n
        detail = i18n.global.t('apiError.notJson', { status: res.status, detail: snippet(text) })
      }
    }
  } catch {
    /* 忽略读取失败，沿用状态码文案 */
  }
  throw new ApiError(res.status, detail)
}

/** 发一个返回 JSON 的请求。 */
export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const init: RequestInit = {
    method: options.method ?? (options.json !== undefined || options.form !== undefined ? 'POST' : 'GET'),
    headers: {},
    signal: options.signal,
  }
  if (options.json !== undefined) {
    ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
    init.body = JSON.stringify(options.json)
  } else if (options.form !== undefined) {
    init.body = options.form
  }

  const res = await fetch(apiURL(path) + buildQuery(options.query), init)
  if (!res.ok) await extractError(res)
  if (res.status === 204) return undefined as T
  const text = await res.text()
  if (!text) return undefined as T
  try {
    return JSON.parse(text) as T
  } catch {
    throw new ApiError(
      res.status,
      i18n.global.t('apiError.notJson', { status: res.status, detail: snippet(text) }),
    )
  }
}

/** 发一个返回纯文本的请求（文件内容、下载等）。 */
export async function requestText(path: string, options: RequestOptions = {}): Promise<string> {
  const res = await fetch(apiURL(path) + buildQuery(options.query), {
    method: options.method ?? 'GET',
    signal: options.signal,
  })
  if (!res.ok) await extractError(res)
  return res.text()
}

/** 带进度回调的上传。 */
export function upload<T>(
  path: string,
  form: FormData,
  onProgress?: (percent: number) => void,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', apiURL(path))
    xhr.upload.addEventListener('progress', (e) => {
      if (e.lengthComputable && onProgress) {
        onProgress(Math.round((e.loaded / e.total) * 100))
      }
    })
    xhr.addEventListener('load', () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText) as T)
        } catch {
          reject(new ApiError(xhr.status, i18n.global.t('apiError.badJson')))
        }
        return
      }
      let message = i18n.global.t('apiError.uploadFailed', { status: xhr.status })
      try {
        const parsed = JSON.parse(xhr.responseText) as { error?: string }
        if (parsed.error) message = parsed.error
      } catch {
        /* 保持默认文案 */
      }
      reject(new ApiError(xhr.status, message))
    })
    xhr.addEventListener('error', () => reject(new ApiError(0, i18n.global.t('apiError.network'))))
    xhr.addEventListener('abort', () => reject(new ApiError(0, i18n.global.t('apiError.aborted'))))
    xhr.send(form)
  })
}

/**
 * 拼一个「原始文件」地址。
 *
 * 给浏览器自己发请求的场景用：下载、图片预览（<img src>）。注意这类请求不会
 * 走 request()，所以后端返回什么就是什么。
 */
export function rawURL(path: string, query?: Record<string, QueryValue>): string {
  return apiURL(path) + buildQuery(query)
}

/** 从 Content-Disposition 或路径里取一个文件名（至少给一个合理的默认值）。 */
function filenameOf(path: string, disposition: string | null): string {
  const quoted = /filename="([^"]+)"/i.exec(disposition ?? '')
  if (quoted?.[1]) return quoted[1]
  const base = path.slice(path.lastIndexOf('/') + 1)
  return base || 'download'
}

/**
 * 触发浏览器下载。
 *
 * 先 fetch 再用 object URL 落地，而不是直接点一个 `<a href>`：后者在**失败**时
 * （目标不存在、网关插了一脚）会把整个文档导航到那个 JSON 错误页——SPA 里的标签和
 * 没保存的编辑全没了，用户只看到一行错误。走 fetch 就能像其它接口一样把原因报出来，
 * 成功时也由我们自己定文件名。
 */
export async function download(path: string, query?: Record<string, QueryValue>): Promise<void> {
  const res = await fetch(rawURL(path, query))
  if (!res.ok) await extractError(res)

  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  try {
    const a = document.createElement('a')
    a.href = url
    a.download = filenameOf(path, res.headers.get('Content-Disposition'))
    a.rel = 'noopener'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
  } finally {
    // 点击之后立刻撤销会让部分浏览器来不及取内容，放到下一轮再说。
    window.setTimeout(() => URL.revokeObjectURL(url), 10_000)
  }
}
