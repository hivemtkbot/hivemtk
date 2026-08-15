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
        extend: true,
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

