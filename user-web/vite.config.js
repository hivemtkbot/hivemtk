import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import VueI18n from '@intlify/unplugin-vue-i18n/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import { resolve } from 'path'

// Element Plus 图标按需自动导入 resolver
//
// 背景：ElementPlusResolver 默认仅匹配 `El*` 前缀组件与 `ElIcon*` 前缀图标,
// 不处理模板里裸用的 `<Edit />`、`<CircleCloseFilled />` 等图标组件名。
// 此处构造一个自定义 resolver,在编译期把模板里裸用的图标名解析为
// `import { IconName } from '@element-plus/icons-vue'`,实现"按需导入"。
//
// 与 main.js 中删除的全量 `app.component(key, component)` 注册循环等价,
// 但仅注入实际被模板引用的图标,显著减小首屏 bundle 体积。
//
const epIconNames = new Set(Object.keys(ElementPlusIconsVue))
function ElementPlusIconResolver(name) {
  // 仅当 name 是 @element-plus/icons-vue 导出的图标名时才解析
  // 跳过 El 前缀(由 ElementPlusResolver 处理)、跳过小写开头(自定义组件)
  if (!/^[A-Z][a-zA-Z0-9]+$/.test(name)) return
  if (!epIconNames.has(name)) return
  return {
    name,
    from: '@element-plus/icons-vue',
  }
}

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
    }),
    // Element Plus 按需自动导入：
    //   - 图标(裸用 <Edit /> 等): 由 ElementPlusIconResolver 解析,从 @element-plus/icons-vue 注入 import
    //   - 组件(<ElXxx />): 由 ElementPlusResolver 解析(此处仍保留 app.use(ElementPlus) 全量注册,
    //     两者并存不冲突,组件自动导入仅作降级路径)
    //   - importStyle:false: 不自动注入组件 CSS(仍走 main.js 中的 'element-plus/dist/index.css' 全量样式,
    //     待 P1 后续阶段再切换为按需 CSS)
    // 参考: https://element-plus.org/en-US/guide/quickstart.html#on-demand-import
    Components({
      resolvers: [
        ElementPlusIconResolver,
        ElementPlusResolver({
          importStyle: false,
        }),
      ],
      dts: 'src/types/components.d.ts',
    }),
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
