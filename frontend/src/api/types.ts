/** 与 Go 侧 internal/server 的响应结构一一对应。 */

export interface FnpackInfo {
  bin: string
  version: string
  message: string
  ok: boolean
}

export interface SystemInfo {
  version: string
  baseURL: string
  socket: string
  pid: number
  pidFile: string
  dataDir: string
  projectsDir: string
  outputDir: string
  logDir: string
  tmpDir: string
  logFile: string
  fnpack: FnpackInfo
  lastSeq: number
  startedAt: string
}

export interface ProjectInfo {
  name: string
  appname: string
  version: string
  displayName: string
  platform: string
  docker: boolean
  ui: boolean
  building: boolean
  modTime: string
}

export interface ProjectExists {
  exists: boolean
  docker: boolean
  ui: boolean
}

export interface PackSettings {
  multiPlatform: boolean
  platforms: string[]
  serverDir: string
  copyToOutput: boolean
}

export interface PackResult {
  platform: string
  ok: boolean
  file: string
  path: string
  size: number
  log: string
}

export interface Artifact {
  name: string
  scope: 'project' | 'output'
  path: string
  size: number
  sizeRaw: string
  modTime: string
}

export interface FsEntry {
  name: string
  dir: boolean
  size: number
  mode: string
  modTime: string
  symlink: boolean
}

export interface FsListing {
  path: string
  abs: string
  entries: FsEntry[]
}

export interface FileContent {
  path: string
  size: number
  sizeRaw: string
  text: boolean
  content: string
}

export interface LogEntry {
  seq: number
  time: string
  level: 'debug' | 'info' | 'warn' | 'error'
  source: string
  msg: string
}
