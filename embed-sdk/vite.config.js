import { defineConfig } from 'vite'
import { resolve } from 'path'

export default defineConfig({
  build: {
    lib: {
      entry: resolve(__dirname, 'src/widget.js'),
      name: 'MarketingChatWidget',
      formats: ['iife', 'esm'],
      fileName: (format) => `marketing-chat-widget.${format}.js`
    },
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // IIFE 模式下用全局变量名
        extend: true,
        // 统一 exports 模式,避免 "named and default together" 警告
        exports: 'named'
      }
    },
    minify: 'esbuild',
    sourcemap: false,
    target: 'es2018'
  },
  server: {
    port: 5174,
    open: '/demo.html'
  }
})
