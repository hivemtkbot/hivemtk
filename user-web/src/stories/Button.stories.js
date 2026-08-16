// =============================================================================
// Button.stories.js — 演示用 Story (OPT-FE-EXT-2)
// =============================================================================
// 这是 Storybook 组件库的"Hello World"。
// 目标:
//   1) 验证 storybook 工具链能跑 (build 成功 + stories 列表可见)
//   2) 演示 CSF 3.0 语法 (Component Story Format 3.0)
//   3) 作为后续迁移 src/components/*.vue 到 stories 的范本
//
// 完整接入路径 (待后续 OPT-FE-EXT-3 实施):
//   - 从 src/components/Breadcrumb.vue 拆出 <MyButton> 风格的标准按钮
//   - 用 actions 演示 click 事件
//   - 用 controls 演示 variant/size/disabled 等 props
// =============================================================================

export default {
  title: 'Demo/Button',
  component: {},  // 骨架阶段为空,真实组件将在 OPT-FE-EXT-3 引入
  tags: ['autodocs'],
  argTypes: {
    label: { control: 'text', description: '按钮文本' },
    variant: {
      control: { type: 'select' },
      options: ['primary', 'default', 'danger'],
      description: '按钮变体',
    },
    size: {
      control: { type: 'inline-radio' },
      options: ['small', 'medium', 'large'],
      description: '按钮尺寸',
    },
    disabled: { control: 'boolean' },
  },
  args: {
    label: 'Hello Storybook',
    variant: 'primary',
    size: 'medium',
    disabled: false,
  },
};

// 模板渲染器 (骨架: 用纯 HTML 模拟 Element Plus 风格按钮)
const Template = (args) => ({
  components: {},
  setup() {
    return { args };
  },
  template: `
    <button
      :class="['demo-btn', 'demo-btn--' + args.variant, 'demo-btn--' + args.size]"
      :disabled="args.disabled"
      @click="$emit('click')"
    >
      {{ args.label }}
    </button>
  `,
});

// 默认 story
export const Default = Template.bind({});
Default.args = {
  label: 'Default Button',
};

// 主按钮
export const Primary = Template.bind({});
Primary.args = {
  label: 'Primary Action',
  variant: 'primary',
};

// 危险按钮
export const Danger = Template.bind({});
Danger.args = {
  label: 'Delete',
  variant: 'danger',
};

// 禁用状态
export const Disabled = Template.bind({});
Disabled.args = {
  label: 'Cannot Click',
  disabled: true,
};
