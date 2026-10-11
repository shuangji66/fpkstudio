import { apiURL } from '@/runtime'

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
        detail = `HTTP ${res.status} 返回的不是 JSON：${snippet(text)}`
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
    throw new ApiError(res.status, `HTTP ${res.status} 返回的不是 JSON：${snippet(text)}`)
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
          reject(new ApiError(xhr.status, '响应不是合法 JSON'))
        }
        return
      }
      let message = `上传失败（${xhr.status}）`
      try {
        const parsed = JSON.parse(xhr.responseText) as { error?: string }
        if (parsed.error) message = parsed.error
      } catch {
        /* 保持默认文案 */
      }
      reject(new ApiError(xhr.status, message))
    })
    xhr.addEventListener('error', () => reject(new ApiError(0, '网络错误')))
    xhr.addEventListener('abort', () => reject(new ApiError(0, '上传已取消')))
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

/** 触发浏览器下载。 */
export function download(path: string, query?: Record<string, QueryValue>): void {
  const a = document.createElement('a')
  a.href = rawURL(path, query)
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}
