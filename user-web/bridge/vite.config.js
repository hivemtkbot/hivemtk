import { defineConfig } from 'vite';

// 仅用于单元测试（vitest）。扩展产物由 scripts/build.mjs 用 esbuild 独立打包。
export default defineConfig({
  test: {
    environment: 'jsdom',
    include: ['test/**/*.test.js'],
  },
});
