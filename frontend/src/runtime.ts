/**
 * 运行时配置。
 *
 * 服务端在返回 index.html 时会注入 window.__FPKSTUDIO__（见 internal/server/static.go）。
 * 前端所有请求 URL 都由此推导，因此同一份构建产物可以挂在 / 也可以挂在
 * /app/fpkstudio 之类的子路径下，不需要重新构建。
 */
interface Injected {
  baseURL?: string
  version?: string
}

const injected: Injected = (window as unknown as { __FPKSTUDIO__?: Injected }).__FPKSTUDIO__ ?? {}

function normalizeBase(input: string | undefined): string {
  if (!input || input === '/') return '/'
  const withLead = input.startsWith('/') ? input : `/${input}`
  const trimmed = withLead.replace(/\/+$/, '')
  return trimmed === '' ? '/' : trimmed
}

/** 应用挂载前缀，形如 `/` 或 `/app/fpkstudio`。 */
export const baseURL: string = normalizeBase(injected.baseURL)

/** 后端注入的版本号，取不到时回落到 dev。 */
export const appVersion: string = injected.version || 'dev'

/** 把站内路径拼成带 baseURL 的绝对路径。 */
export function apiURL(path: string): string {
  const p = path.startsWith('/') ? path : `/${path}`
  return baseURL === '/' ? p : `${baseURL}${p}`
}
