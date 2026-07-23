import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import VueI18n from '@intlify/unplugin-vue-i18n/vite'
import { resolve } from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    // 预编译 i18n 资源（./src/i18n/locales/*.json），运行期不再编译消息，
    // 避免 vue-i18n 用 new Function 触发 CSP script-src 'self' 的 unsafe-eval 拦截。
    // runtimeOnly 默认 true：改用 vue-i18n 运行时构建（无消息编译器）。
    // strictMessage:false 因为部分翻译含 <g>/<x>/<string> 等占位标签，按字面量保留。
    VueI18n({
      include: resolve(__dirname, './src/i18n/locales/**'),
      strictMessage: false,
      dropMessageCompiler: true,
    }),
    vue({
      compilerOptions: {
        // 禁用深度选择器弃用警告
        isCustomElement: () => false,
        whitespace: 'condense',
        // 添加自定义编译器选项以禁用警告
        compatConfig: {
          MODE: 3
        }
      }
    })
  ],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  css: {
    preprocessorOptions: {
      scss: {
        // 移除自动导入，改用@use语法
      }
    },
    // 添加CSS配置以处理深度选择器警告
    devSourcemap: true
  },
  // 添加构建选项以禁用警告
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 1600,
    rollupOptions: {
      output: {
        manualChunks: {
          vue: ['vue', 'vue-router', 'pinia'],
          elementPlus: ['element-plus'],
          utils: ['axios']
        }
      }
    }
  },
  // 添加服务器配置以禁用警告
  server: {
    port: 8211,
    strictPort: false, // 端口被占用时自动递增
    host: true, // 允许外部访问
    open: false, // 不自动打开浏览器
    hmr: {
      overlay: false
    },
    proxy: {
      '/api': {
        target: 'http://localhost:8204',
        changeOrigin: true,
        secure: false,
        ws: true
      }
    }
  },
  // 添加优化选项
  optimizeDeps: {
    include: ['vue', 'vue-router', 'pinia', 'element-plus', 'axios']
  },
  // 添加自定义配置以禁用警告
  define: {
    __VUE_OPTIONS_API__: true,
    __VUE_PROD_DEVTOOLS__: false,
    'process.env': {}
  }
})
