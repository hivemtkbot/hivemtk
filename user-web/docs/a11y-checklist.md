# 无障碍（a11y）开发规范

> 任务编号：**OPT-FE-13**
> 适用范围：[hivemtk/user-web/src/](../../user-web/src/)、[hivemtk/embed-sdk/src/](../../embed-sdk/src/)
> 遵循标准：**WCAG 2.1 AA**

---

## 1. 工具链

| 工具 | 角色 | 状态 |
|------|------|------|
| [eslint-plugin-vuejs-accessibility](https://github.com/vue-a11y/eslint-plugin-vuejs-accessibility) | 静态检查：缺 alt / aria-label / 错误 role 等 | 已加入 `devDependencies` |
| `@axe-core/playwright`（建议） | E2E 扫描：核心页面 axe 扫描 | 后续 PR 引入 |
| 浏览器 DevTools → Accessibility Tab | 调试辅助树 / 对比度 | 开发期使用 |

> ESLint 插件仅声明依赖，**不** 自动启用；CI 改造时通过 `npm i` 一次性安装并接入 `eslint.config.mjs`。
> 接入草稿：

```js
// eslint.config.mjs
import vueA11y from 'eslint-plugin-vuejs-accessibility'
export default [
  // ... 既有配置
  {
    files: ['src/**/*.vue'],
    plugins: { 'vuejs-accessibility': vueA11y },
    rules: {
      'vuejs-accessibility/alt-text': 'warn',
      'vuejs-accessibility/aria-role': 'error',
      'vuejs-accessibility/click-events-have-key-events': 'warn',
      'vuejs-accessibility/label-has-for': 'warn',
      'vuejs-accessibility/no-autofocus': 'warn',
    },
  },
]
```

---

## 2. 组件级 Checklist

### 2.1 通用

- [ ] 所有可交互元素（`<button>` / `<a>` / 自定义 `div+click`）可通过 Tab 聚焦
- [ ] 自定义按钮使用 `<button>` 而非 `<div @click>`；非按钮场景添加 `role="button"` + `tabindex="0"` + 键盘事件
- [ ] 图标按钮必带 `aria-label`（中文）
- [ ] 弹窗（Dialog / Drawer）有 `aria-modal="true"`、关闭后焦点回归触发元素
- [ ] 表单控件有关联的 `<label>` 或 `aria-label` / `aria-labelledby`

### 2.2 图片 / SVG

- [ ] 信息性图片：`alt="描述性文本"`
- [ ] 装饰性图片：`alt=""` 或 `role="presentation"`
- [ ] 内嵌 SVG：复杂图形配 `<title>` / `<desc>`

### 2.3 颜色 / 对比度

- [ ] 正文文本与背景对比度 ≥ 4.5:1
- [ ] 大字号（18pt+ 或 14pt 粗体）对比度 ≥ 3:1
- [ ] 不仅靠颜色传达信息（错误状态需额外 icon / 文字）

### 2.4 键盘

- [ ] 全部功能可纯键盘完成
- [ ] 焦点可见（`:focus-visible` 样式不能被 `outline: none` 覆盖）
- [ ] 模态框打开时禁用背后内容 Tab 导航（focus trap）

---

## 3. 已审计的核心组件

| 组件 | 修复内容 | 状态 |
|------|---------|------|
| `embed-sdk/src/floating-button.js` | 已有 `aria-label="打开在线客服"`；已加 `aria-expanded` | ✅ |
| `user-web/src/components/PageHeader.vue` | 加 `role="banner"` + `aria-label` | ✅ |
| `user-web/src/components/Breadcrumb.vue` | 加 `aria-label="面包屑导航"` | ✅ |
| `user-web/src/components/PageState.vue` | 加 `role="status"` / `role="alert"` + `aria-live` | ✅ |

---

## 4. PR 验收清单（合并前必填）

- [ ] 跑 `npm run lint:check` 无新增 a11y 报错
- [ ] 涉及 Dialog / Drawer / 自定义焦点组件的改动附 DevTools Accessibility 截图
- [ ] 表单字段使用 `<el-form-item label="...">` 或 `aria-label`
- [ ] 图标按钮均带 `aria-label`
- [ ] 键盘 Tab 链路走通新功能

---

## 5. 后续路线图

1. 引入 `@axe-core/playwright`，核心页面 e2e 扫描加入 CI
2. 抽取 `useFocusTrap()` 通用 composable
3. 屏幕阅读器实测（VoiceOver / NVDA）覆盖关键业务流
4. 第三方贡献组件（Element Plus / ECharts）已知 a11y 限制登记到本文件附录

## 6. 引用

- [WCAG 2.1 AA 速查](https://www.w3.org/WAI/WCAG21/quickref/?currentcoloride=levels-aaa)
- [Vue a11y 官方指南](https://vuejs.org/guide/best-practices/accessibility.html)
