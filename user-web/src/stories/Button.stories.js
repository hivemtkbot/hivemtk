export default {
  title: 'Demo/Button',
  component: {},
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
  }
};

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

export const Default = Template.bind({});
Default.args = {
  label: 'Default Button',
};

export const Primary = Template.bind({});
Primary.args = {
  label: 'Primary Action',
  variant: 'primary',
};

export const Danger = Template.bind({});
Danger.args = {
  label: 'Delete',
  variant: 'danger',
};

export const Disabled = Template.bind({});
Disabled.args = {
  label: 'Cannot Click',
  disabled: true,
};
