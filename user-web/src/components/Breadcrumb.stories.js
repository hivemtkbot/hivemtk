import Breadcrumb from './Breadcrumb.vue'

export default {
  title: 'Components/Breadcrumb',
  component: Breadcrumb,
  tags: ['autodocs'],
  argTypes: {},
  args: {},
}

const Template = (args) => ({
  components: { Breadcrumb },
  setup() {
    return { args }
  },
  template: '<Breadcrumb v-bind="args" />',
})

export const Default = Template.bind({});
Default.args = {}

export const HomePage = Template.bind({});
HomePage.parameters = {
  docs: {
    description: {
      story: '首页状态的面包屑导航',
    },
  },
}

export const SubPage = Template.bind({});
SubPage.parameters = {
  docs: {
    description: {
      story: '二级页面状态的面包屑导航',
    },
  },
}
