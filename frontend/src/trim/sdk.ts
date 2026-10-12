/**
 * 飞牛（fnOS）宿主能力的薄封装。
 *
 * 为什么要包一层：`@trimjs/web-app` 是宿主注入的微应用 SDK，页面既可能跑在
 * 飞牛的应用 iframe 里，也可能被人直接用浏览器打开（本项目的后端只监听 unix
 * socket，很多人是桥出来自己点的）。两种情况差别只在「有没有宿主桥」：
 *
 *   - 跑在宿主里（`isStandaloneWeb === false`）：文件选择、授权、读主题都能直接调；
 *   - 独立浏览器页面（`isStandaloneWeb === true`）：这些能力一律没有，调了只会
 *     抛「Host bridge is not available」。所以这里统一先判环境，调用方只管
 *     处理「不可用」这一个分支。
 *
 * 官方推荐的另一条路是 `openAppAuth`（打开系统授权页 + redirectUri 回调），它
 * 要求应用挂在统一网关下、回调页与原页面同源。本项目的部署形态是「反代到
 * socket」，没有这条链路，所以独立浏览器里干脆不提供入口、明确告诉用户去飞牛
 * 里打开，而不是给一个点了就报错的按钮。
 *
 * 后端接口（trim.file.checkUserACL / convertPath / getUserAccessibleFolders）
 * 不在这里调：它们只能由服务端经 unix socket 调，前端走 /api/trim/*。
 */
import { TrimApp } from '@trimjs/web-app'
import { i18n } from '@/i18n'

/** 文件选择器左侧的分组（官方枚举）。 */
export type SidebarGroup = 'myFiles' | 'otherShare' | 'external' | 'remote' | 'favorites' | 'team'

/**
 * 选择器要显示的分组：**除了远程挂载和他人分享，其余都列出来**。
 *
 * 「远程挂载」里常常是没挂好的网盘 / SMB，「他人分享」不是当前用户自己的文件，
 * 两者拿来当项目根或 fpk 输出目录都不合适，还容易选错。
 */
export const SIDEBAR_GROUPS: SidebarGroup[] = ['myFiles', 'external', 'favorites', 'team']

export interface PickParams {
  /** 选目录（目录授权只支持单选），默认选文件。 */
  directory?: boolean
  accept?: string[]
  title?: string
  okText?: string
}

/** 宿主能力调用失败（业务码非 0）。 */
export class TrimError extends Error {
  readonly code: number

  constructor(code: number, message: string) {
    super(message)
    this.name = 'TrimError'
    this.code = code
  }
}

/**
 * 宿主能力用不了。
 *
 * 两种情况分开报：独立浏览器页面（压根没有宿主桥），以及宿主桥迟迟不完备
 * （页面在 iframe 里，但对面不是飞牛宿主）。后者的表现很坑——SDK 的初始化
 * Promise 永远不 settle，文件选择器调用会一直挂着，界面只能一直转圈，
 * 所以下面用 ensureHostReady 主动等一个上限。
 */
export class TrimUnavailableError extends Error {
  readonly reason: 'standalone' | 'notReady'

  constructor(reason: 'standalone' | 'notReady' = 'standalone') {
    super(reason === 'standalone' ? 'not running inside the fnOS host' : 'host bridge is not ready')
    this.name = 'TrimUnavailableError'
    this.reason = reason
  }
}

/** 等宿主桥就绪的上限。正常宿主里初始化是百毫秒级，这里给足余量。 */
const HOST_READY_TIMEOUT_MS = 10000

/**
 * 确认宿主桥真的可用。
 *
 * 只判 `isStandaloneWeb` 是不够的：页面被塞进一个普通 iframe（比如有人在自建
 * 页面上嵌了本应用）时，`isStandaloneWeb` 是 false，但对面根本不回应握手，
 * SDK 的初始化 Promise 永远不 resolve——`pickUserFile` 会挂死在那儿。
 */
async function ensureHostReady(): Promise<void> {
  if (!isHostAvailable()) throw new TrimUnavailableError('standalone')

  let timer: number | undefined
  const timeout = new Promise<never>((_, reject) => {
    timer = window.setTimeout(() => reject(new TrimUnavailableError('notReady')), HOST_READY_TIMEOUT_MS)
  })
  try {
    await Promise.race([sdk.ready(), timeout])
  } finally {
    if (timer !== undefined) window.clearTimeout(timer)
  }
}

const sdk = new TrimApp()

/**
 * 宿主能力是否可用。
 *
 * 只判 `isStandaloneWeb`：移动端 App 内嵌页面（`isWeb === false`）同样支持文件
 * 选择与授权，把它一起挡掉没有道理。
 */
export function isHostAvailable(): boolean {
  return !sdk.isStandaloneWeb
}

/**
 * 拉起用户文件选择器，并把选中的路径授权给本应用。
 *
 * 返回的 `data` 是「当前用户授权给本应用的路径」。目录授权会返回已授权的目录，
 * 文件授权返回这次选中的文件——所以选目录时调用方要自己 diff 出新增的那一个
 * （见 SettingsDialog 的说明）。
 *
 * **空数组代表用户取消了**（没选、也没授权），不是错误。宿主在用户直接关掉选择器时
 * 返回的是 `code: 1` + `msg: "Operation failed"`——一个既没有业务码、也没有可读
 * 原因的信号，当错误抛出去只会让界面弹一句「调用飞牛接口失败（1）：Operation failed」，
 * 而用户明明只是点了取消。所以这里把 `code: 1` 与「成功但没返回路径」都归一成空数组，
 * 由调用方给一句「已取消」的提示。
 *
 * 只把 `code: 1` 当取消是安全的：文档里另一种 `code: 1`（「仅管理员可进行此操作」）
 * 出现在**应用共享授权**入口，而这里调的是用户个人授权 `pickUserFile`。真正没有
 * 权限的情况会走 `1003201`「管理员已关闭该应用的普通用户授权能力」，仍然是错误。
 */
async function pickUserPaths(params: PickParams): Promise<string[]> {
  await ensureHostReady()

  const res = await sdk.pickUserFile({
    directory: params.directory === true,
    accept: params.accept,
    sidebarGroup: SIDEBAR_GROUPS,
    title: params.title,
    okText: params.okText,
  })

  if (!res) throw new TrimError(-1, i18n.global.t('trim.error.noSelection'))
  if (res.code === 1) return []
  if (res.code !== 0) throw new TrimError(res.code, res.msg || `code ${res.code}`)
  return res.data ?? []
}

/** 选一个目录；返回授权结果里的全部目录路径（调用方自己挑新增的那个）。 */
export const pickUserDirectory = (title: string, okText: string): Promise<string[]> =>
  pickUserPaths({ directory: true, title, okText })

/** 选一个文件（用 accept 限定扩展名）；返回授权结果里的全部文件路径。 */
export const pickUserFiles = (
  title: string,
  okText: string,
  accept: string[],
): Promise<string[]> => pickUserPaths({ directory: false, accept, title, okText })

/**
 * 在飞牛的文件管理器里打开某个目录（官方「页面路由」的 `openFileManager`）。
 *
 * 这个能力没有 scope 要求，但仍然是宿主能力：独立浏览器页面里没有文件管理器可开，
 * 所以先判环境、再等宿主桥就绪，失败原因交给调用方展示。
 */
export async function openInFileManager(path: string): Promise<void> {
  await ensureHostReady()
  await sdk.openFileManager(path)
}

/**
 * 读宿主当前主题。
 *
 * 独立浏览器页面读不到（没有宿主），返回 null，由调用方回落到
 * `prefers-color-scheme`。失败同样返回 null——主题读不到不该影响页面。
 */
export async function hostTheme(): Promise<'light' | 'dark' | null> {
  if (!isHostAvailable()) return null
  try {
    // 必须先等宿主桥就绪（有 10 秒上限）。只判 isStandaloneWeb 是不够的：页面被塞进
    // 一个普通 iframe 时它是 false，而对面的握手永远不回应，`getPlatformConfig()`
    // 会一直挂着——主题初始化就停在这一步，`apply()` 永远轮不到，表现是「本地存着
    // 深色，刷新之后页面还是浅色的」。
    await ensureHostReady()
    const config = await sdk.getPlatformConfig()
    return config.theme === 'dark' ? 'dark' : 'light'
  } catch {
    return null
  }
}

/**
 * 订阅宿主主题变化。
 *
 * `$on` 只支持 Web 宿主环境（见官方文档「页面交互」），移动端 App 内嵌页面和独立
 * 浏览器页面都不支持，所以在这些环境里直接不订阅，返回 false。
 */
export async function onHostThemeChange(cb: (theme: 'light' | 'dark') => void): Promise<boolean> {
  if (!sdk.isWeb || sdk.isStandaloneWeb) return false
  try {
    await sdk.$on('os/theme', (theme) => {
      cb(theme === 'dark' ? 'dark' : 'light')
    })
    return true
  } catch {
    return false
  }
}
