import { fileURLToPath } from 'node:url'

import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

const entry = (name: string) => fileURLToPath(new URL(`./${name}`, import.meta.url))

// 三个页面各是一个入口，构建产物就是 index.html / auth.html / maintain.html 加一堆带哈希的
// assets/：URL 与路由的对应关系由根目录的 web.go 定（/ → index.html，/auth → auth.html）。
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  build: {
    outDir: 'dist',
    rollupOptions: {
      input: {
        index: entry('index.html'),
        auth: entry('auth.html'),
        maintain: entry('maintain.html'),
      },
    },
  },
  // 开发时页面由 Vite 自己出，接口仍打 Go（just run 起 8080）：改前端不用重新编译二进制。
  server: {
    proxy: Object.fromEntries(
      ['/api', '/asset', '/livez', '/readyz'].map((path) => [path, 'http://127.0.0.1:8080']),
    ),
  },
})
