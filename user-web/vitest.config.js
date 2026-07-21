import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config.js'

// Vitest 配置：单测 + 组件测试
// 复用 vite.config.js 的 alias 与插件配置
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      globals: true,
      environment: 'jsdom',
      include: ['tests/unit/**/*.{test,spec}.{js,mjs,ts}'],
      exclude: ['node_modules', 'dist', 'tests/e2e/**'],
      coverage: {
        provider: 'v8',
        reporter: ['text', 'html', 'json-summary'],
        reportsDirectory: './coverage',
        include: [
          'src/utils/**',
          'src/api/backup.js',
          'src/api/order.js',
          'src/api/securityAudit.js',
          'src/views/backup/**',
          'src/views/order/**',
          'src/views/securityAudit/**'
        ],
        exclude: ['**/*.test.*', '**/*.spec.*', '**/__tests__/**']
      }
    }
  })
)
