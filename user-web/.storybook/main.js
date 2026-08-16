/** @type { import('@storybook/vue3-vite').StorybookConfig } */
// =============================================================================
// Storybook 主配置 (OPT-FE-EXT-2)
// 创建日期: 2026-08-16
// 目标: user-web 组件库独立开发环境 + 视觉回归基线
//
// 注意: 完整功能需先 `npm i -D @storybook/vue3-vite @storybook/addon-essentials`
// 本文件仅骨架, 在依赖安装前无法直接 `npm run storybook`
// =============================================================================

const config = {
  // stories 扫描路径: 匹配 src/stories 与 src/components 下所有 *.stories.{js,ts}
  stories: [
    '../src/stories/**/*.stories.@(js|ts|mdx)',
    '../src/components/**/*.stories.@(js|ts|mdx)',
  ],

  // Vue 3 + Vite 构建器
  framework: {
    name: '@storybook/vue3-vite',
    options: {},
  },

  // 静态资源目录
  staticDirs: ['../public'],

  // 文档化插件(MDX 支持, 默认开)
  addons: [
    '@storybook/addon-essentials',
    '@storybook/addon-interactions',
    '@storybook/addon-a11y',
  ],

  // 开发服务器配置
  core: {
    disableTelemetry: true,  // 关闭遥测
  },

  // TypeScript 配置
  typescript: {
    check: false,  // 不在 storybook 启动时跑 tsc
    reactDocgen: false,
  },

  // 文档入口配置
  docs: {},
};

export default config;
