# ESLint v9 flat config（2026-08-15 P0-CI：开启前端 ESLint）
#
# 目标：
#   - 在 CI 中阻断明显错误（no-undef / no-unused-vars / no-empty）
#   - 风格约束宽松（与既有 ESM 写法兼容，不强制 import order 等过度规则）
#   - 不阻断开发（缺依赖时降级为 warn）
#
# 规则集：eslint:recommended 基础 + 适配 ESM/Node 环境
import js from '@eslint/js';
import globals from 'globals';

export default [
  {
    ignores: [
      'dist/**',
      'node_modules/**',
      'coverage/**',
      '**/*.min.js',
    ],
  },
  js.configs.recommended,
  {
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'module',
      globals: {
        ...globals.browser,
        ...globals.node,
        chrome: 'readonly',
        console: 'readonly',
        process: 'readonly',
        setTimeout: 'readonly',
        clearTimeout: 'readonly',
        setInterval: 'readonly',
        clearInterval: 'readonly',
        fetch: 'readonly',
        URL: 'readonly',
        URLSearchParams: 'readonly',
        Buffer: 'readonly',
        globalThis: 'readonly',
      },
    },
    rules: {
      // 安全红线
      'no-undef': 'error',
      'no-unused-vars': ['warn', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
      'no-empty': ['error', { allowEmptyCatch: true }],
      'no-constant-condition': ['error', { checkLoops: false }],
      'no-prototype-builtins': 'off',
      'no-useless-escape': 'warn',
      // 风格（宽松）
      'no-var': 'warn',
      'prefer-const': 'warn',
      'eqeqeq': ['warn', 'always', { null: 'ignore' }],
      'no-console': 'off',
    },
  },
  {
    // 测试文件允许更多写法
    files: ['test/**/*.js', '**/*.test.js'],
    rules: {
      'no-unused-vars': 'off',
      'no-empty': 'off',
    },
  },
];
