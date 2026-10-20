import { apiURL } from '@/runtime'
import { download, request, requestText, upload } from './client'
import type {
  Artifact,
  ExtractResult,
  FileContent,
  FsListing,
  LogEntry,
  PackResult,
  PackSettings,
  ProjectExists,
  ProjectInfo,
  SemanticPath,
  SettingsInfo,
  SystemInfo,
} from './types'

// ---- 系统 ----

export const getSystemInfo = () => request<SystemInfo>('/api/system/info')

// ---- 设置（目录 + 飞牛开放 API 的转发） ----
//
// checkUserACL 没有前端的调用方：ACL 是后端在导入前判当前用户能不能读那个文件，
// 界面上不展示权限标签（只展示 trim.file.path 转出来的展示路径）。

export const getAppSettings = () => request<SettingsInfo>('/api/settings')

/**
 * 保存两个目录设置。只传要改的那个字段，另一个保持不变。
 * 空字符串表示「恢复默认」（用环境变量给的目录）。
 */
export const saveAppSettings = (patch: { projectRoot?: string; outputDir?: string }) =>
  request<SettingsInfo>('/api/settings', { method: 'PUT', json: patch })

/** 当前用户授权给本应用的目录（不含文件授权）。 */
export const getAuthorizedFolders = async (): Promise<string[]> => {
  const res = await request<{ paths: string[] }>('/api/trim/folders')
  return res.paths ?? []
}

/** 删除一条目录授权：换目录前必须先删旧的，否则授权会一份份堆在系统里。 */
export const deleteAuthorizedFolder = (path: string) =>
  request<{ ok: boolean }>('/api/trim/folders/delete', { json: { path } })

/**
 * 把 /vol1/... 这类内部路径转成用户能看懂的语义路径。
 *
 * language 放 body 而不是查询串（和 path 一样）：反代不保证把查询串送到后端，
 * 而这是文档要求的必填参数，丢了系统就会按别的语言返回。
 */
export const convertPaths = async (paths: string[], language: string): Promise<SemanticPath[]> => {
  const res = await request<{ list: SemanticPath[] }>('/api/trim/semantic', {
    json: { paths, language },
  })
  return res.list ?? []
}

/** 导入服务器本机某个路径上的 fpk（飞牛文件选择器选出来的）。 */
export const importProjectByPath = (path: string) =>
  request<{ ok: boolean; project: ProjectInfo }>('/api/projects/import-path', { json: { path } })

// ---- 项目 ----

export async function listProjects(): Promise<ProjectInfo[]> {
  const res = await request<{ list: ProjectInfo[]; count: number }>('/api/projects')
  return res.list ?? []
}

export const projectExists = (name: string) =>
  request<ProjectExists>('/api/projects/exists', { query: { name } })

export const createProject = (name: string, docker: boolean, ui: boolean) =>
  request<{ ok: boolean; project?: ProjectInfo }>('/api/projects/create', {
    json: { name, docker, ui },
  })

export const deleteProject = (name: string) =>
  request<{ ok: boolean }>('/api/projects/delete', { json: { name } })

export const getPlatforms = async (name: string): Promise<string[]> => {
  const res = await request<{ platforms: string[] }>('/api/projects/platforms', { query: { name } })
  return res.platforms ?? []
}

/**
 * 打包偏好。
 *
 * `platforms` 在这里兜一次底：后端（以及早期写下的 settings.json）在架构为空时
 * 会给 `null`，而调用方会直接展开 / 遍历它。老接口的返回不保证是数组，所以
 * 在边界上统一成数组，别让 `[...null]` 把整个页面中途打断。
 */
export const getSettings = async (name: string): Promise<PackSettings> => {
  const res = await request<PackSettings>('/api/projects/settings', { query: { name } })
  return { ...res, platforms: res.platforms ?? [] }
}

export const saveSettings = (name: string, settings: PackSettings) =>
  request<{ ok: boolean }>('/api/projects/settings', { json: { name, settings } })

export const getArtifacts = async (
  name: string,
): Promise<{ list: Artifact[]; outputDir: string }> => {
  const res = await request<{ list: Artifact[]; outputDir: string }>('/api/projects/artifacts', {
    query: { name },
  })
  return { list: res.list ?? [], outputDir: res.outputDir }
}

/** 删除一条产物：scope 是列表里的 project / output，file 是产物文件名。 */
export const deleteArtifact = (name: string, file: string, scope: string) =>
  request<{ ok: boolean }>('/api/projects/artifacts/delete', {
    json: { name, file, scope },
  })

export const packProject = (
  name: string,
  platforms: string[],
  serverDir: string,
  copyOut: boolean,
) =>
  request<{ ok: boolean; results: PackResult[] }>('/api/projects/pack', {
    json: { name, platforms, serverDir, copyOut },
  })

export const importProject = (
  file: File,
  onProgress?: (percent: number) => void,
  signal?: AbortSignal,
) => {
  const form = new FormData()
  form.append('file', file)
  return upload<{ ok: boolean; project: ProjectInfo }>('/api/projects/import', form, onProgress, signal)
}

// ---- 文件 ----

/**
 * 读接口的 path 走 JSON body，而不是查询串。
 *
 * 后端两种都认（见 server.pathParam）。不用查询串的原因：反代 / 网关不保证把
 * 查询串原样送到后端，path 一丢就退化成「列项目根目录」或直接报错，表现为文件树
 * 永远是空的、还看不出原因。POST + JSON body 这条路是稳的——所有写接口都走它。
 */
export const listDir = (path: string) =>
  request<FsListing>('/api/fs/list', { method: 'POST', json: { path } })

export const readFile = (path: string) =>
  request<FileContent>('/api/fs/read', { method: 'POST', json: { path } })

export const writeFile = (path: string, content: string) =>
  request<{ ok: boolean; path: string; size: number }>('/api/fs/write', {
    json: { path, content },
  })

export const createEntry = (path: string, dir: boolean) =>
  request<{ ok: boolean }>('/api/fs/create', { json: { path, dir } })

export const deleteEntry = (path: string, dir: boolean) =>
  request<{ ok: boolean }>('/api/fs/delete', { json: { path, dir } })

export const renameEntry = (path: string, to: string) =>
  request<{ ok: boolean }>('/api/fs/rename', { json: { path, to } })

export const copyEntry = (path: string, to: string) =>
  request<{ ok: boolean }>('/api/fs/copy', { json: { path, to } })

/**
 * 解压到压缩包自己所在的目录。
 *
 * 实际解压由后端调系统 tar / gzip 完成（见 `fsx.Extract`），同名文件一律跳过、
 * 不覆盖已有内容，所以返回里的 `skipped` 要告诉用户。
 */
export const extractEntry = (path: string) =>
  request<ExtractResult>('/api/fs/extract', { json: { path } })

export const uploadFiles = (
  dir: string,
  files: File[],
  overwrite: boolean,
  onProgress?: (percent: number) => void,
  signal?: AbortSignal,
) => {
  const form = new FormData()
  form.append('path', dir)
  form.append('overwrite', overwrite ? 'true' : 'false')
  for (const f of files) form.append('file', f, f.name)
  return upload<{ ok: boolean; files: { path: string; size: number }[] }>(
    '/api/fs/upload',
    form,
    onProgress,
    signal,
  )
}

/**
 * 下载文件 / 目录。
 *
 * `package: true` 走 `layout=package`：后端按 fpk 包的白名单只导出 manifest、图标和
 * app / cmd / config / wizard / i18n 这几个脚手架目录（首页「导出」用它），
 * building 工作区、.git、README 之类都不进 zip。普通下载不传，行为不变。
 */
export const downloadEntry = (path: string, opts: { package?: boolean } = {}) =>
  download('/api/fs/download', {
    path,
    layout: opts.package ? 'package' : undefined,
  })

// ---- 日志 ----

/**
 * 增量拉日志。
 *
 * `boot` 是后端进程标识：seq 是进程内计数，重启后从 1 开始，前端靠它决定要不要把
 * 去重游标归零（见 stores/logs.ts）。
 */
export const getLogs = (after: number, limit = 1000, source = '') =>
  request<{ list: LogEntry[]; lastSeq: number; boot: string }>('/api/logs', {
    query: { after, limit, source },
  })

export const logStreamURL = (after: number, source = '') => {
  const q = new URLSearchParams({ after: String(after) })
  if (source) q.set('source', source)
  return apiURL('/api/logs/stream') + `?${q.toString()}`
}

export { requestText }
