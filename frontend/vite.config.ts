import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// 产物直接落到 web/dist，由 Go 的 //go:embed all:web/dist 打进二进制。
// base 固定为 './'：资源用相对路径，服务端再通过注入 <base href> 决定实际挂载点。
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  base: './',
  resolve: {
    // Vite 会把以 / 开头的别名按「项目根目录」解析，写成 /src 即可，
    // 不需要引入 node:url，也就少一个 @types/node 依赖。
    alias: { '@': '/src' },
  },
  server: {
    port: 5273,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        // Vite 8 底层换成 rolldown，manualChunks 只接受函数形式。
        manualChunks(id: string) {
          if (!id.includes('node_modules')) return undefined
          if (id.includes('element-plus')) return 'element'
          if (id.includes('codemirror') || id.includes('@lezer')) return 'codemirror'
          if (id.includes('xterm')) return 'xterm'
          if (id.includes('vue') || id.includes('pinia')) return 'vendor'
          return undefined
        },
      },
    },
  },
})
