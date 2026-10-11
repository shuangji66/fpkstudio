import { apiURL } from '@/runtime'
import { download, request, requestText, upload } from './client'
import type {
  Artifact,
  FileContent,
  FsListing,
  LogEntry,
  PackResult,
  PackSettings,
  ProjectExists,
  ProjectInfo,
  SystemInfo,
} from './types'

// ---- 系统 ----

export const getSystemInfo = () => request<SystemInfo>('/api/system/info')

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

export const getSettings = (name: string) =>
  request<PackSettings>('/api/projects/settings', { query: { name } })

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

export const packProject = (
  name: string,
  platforms: string[],
  serverDir: string,
  copyOut: boolean,
) =>
  request<{ ok: boolean; results: PackResult[] }>('/api/projects/pack', {
    json: { name, platforms, serverDir, copyOut },
  })

export const importProject = (file: File, onProgress?: (percent: number) => void) => {
  const form = new FormData()
  form.append('file', file)
  return upload<{ ok: boolean; project: ProjectInfo }>('/api/projects/import', form, onProgress)
}

// ---- 文件 ----

export const listDir = (path: string) => request<FsListing>('/api/fs/list', { query: { path } })

export const readFile = (path: string) => request<FileContent>('/api/fs/read', { query: { path } })

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

export const uploadFiles = (
  dir: string,
  files: File[],
  overwrite: boolean,
  onProgress?: (percent: number) => void,
) => {
  const form = new FormData()
  form.append('path', dir)
  form.append('overwrite', overwrite ? 'true' : 'false')
  for (const f of files) form.append('file', f, f.name)
  return upload<{ ok: boolean; files: { path: string; size: number }[] }>(
    '/api/fs/upload',
    form,
    onProgress,
  )
}

export const downloadEntry = (path: string) => download('/api/fs/download', { path })

// ---- 日志 ----

export const getLogs = (after: number, limit = 1000, source = '') =>
  request<{ list: LogEntry[]; lastSeq: number }>('/api/logs', {
    query: { after, limit, source },
  })

export const logStreamURL = (after: number, source = '') => {
  const q = new URLSearchParams({ after: String(after) })
  if (source) q.set('source', source)
  return apiURL('/api/logs/stream') + `?${q.toString()}`
}

export { requestText }
