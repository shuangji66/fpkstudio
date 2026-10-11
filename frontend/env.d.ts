/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>
  export default component
}

// 由服务端在 index.html 里注入。
interface FpkstudioRuntime {
  baseURL: string
  version: string
}

interface Window {
  __FPKSTUDIO__?: FpkstudioRuntime
}
