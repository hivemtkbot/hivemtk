<template>
  <div class="guide-container">
    <h1 class="guide-title">{{ $t('邮件营销使用指南') }}</h1>
    <div class="guide-intro">
      <p>欢迎使用邮件营销服务！本指南将引导您完成从 SMTP 配置到邮件发送的完整流程。</p>
    </div>

    <el-steps :active="activeStep" finish-status="success" align-center class="steps-container">
      <el-step :title="$t('配置 SMTP')" description="配置发件邮箱" />
      <el-step :title="$t('创建收件人列表')" description="导入收件人" />
      <el-step :title="$t('编写邮件草稿')" description="准备邮件内容" />
      <el-step :title="$t('发送任务')" description="启动发送并查看进度" />
    </el-steps>

    <el-row :gutter="20" class="step-cards">
      <el-col :span="6" v-for="(item, idx) in steps" :key="idx">
        <el-card shadow="hover" class="step-card" :class="{ active: activeStep === idx }" @click="activeStep = idx">
          <h3>{{ item.title }}</h3>
          <p class="desc">{{ item.description }}</p>
          <ul>
            <li v-for="(d, i) in item.details" :key="i">{{ d }}</li>
          </ul>
          <el-button type="primary" link @click.stop="goTo(item.path)">
            {{ $t('前往操作') }} <el-icon><ArrowRight /></el-icon>
          </el-button>
        </el-card>
      </el-col>
    </el-row>

    <el-alert :title="$t('常见问题')" type="info" :closable="false" show-icon class="faq">
      <p><strong>Q: SMTP 验证失败？</strong> 请检查发件人邮箱、授权码是否正确，并确认邮箱已开启 SMTP 服务。</p>
      <p><strong>Q: 邮件进入垃圾箱？</strong> {{ $t('建议使用企业域名邮箱，避免营销话术，并合理设置发送频率。') }}</p>
      <p><strong>Q: 发送速度太慢？</strong> {{ $t('适当增加任务并发数，但需避免触发反垃圾邮件规则。') }}</p>
    </el-alert>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const activeStep = ref(0)

const steps = [
  {
    title: i18n.global.t('1. 配置 SMTP'),
    description: '配置发件人邮箱',
    details: [
      '进入"邮件营销 → SMTP 配置"',
      '填写发件邮箱、授权码、服务器地址',
      '点击"测试连接"验证配置'
    ],
    path: '/email/smtp'
  },
  {
    title: i18n.global.t('2. 收件人列表'),
    description: '导入和管理收件人',
    details: [
      '进入"邮件营销 → 收件人列表"',
      '手动添加或导入 CSV/Excel 文件',
      '为收件人打标签,便于分组发送'
    ],
    path: '/email/list'
  },
  {
    title: i18n.global.t('3. 编写草稿'),
    description: '准备邮件内容',
    details: [
      '进入"邮件营销 → 邮件草稿"',
      '编写邮件主题与正文',
      '使用模板变量 {name} {phone} 等'
    ],
    path: '/email/drafts'
  },
  {
    title: i18n.global.t('4. 发送任务'),
    description: '创建并启动发送任务',
    details: [
      '进入"邮件营销 → 发送任务"',
      '选择草稿、收件人列表',
      '配置发送时间与并发数,启动任务'
    ],
    path: '/email/jobs'
  }
]

const goTo = (path) => {
  router.push(path).catch(() => {})
}
</script>

<style scoped>
.guide-container {
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px;
}
.guide-title {
  text-align: center;
  margin-bottom: 20px;
  color: #333;
}
.guide-intro {
  background-color: #f0f7ff;
  padding: 15px 20px;
  border-radius: 8px;
  margin-bottom: 20px;
  border: 1px solid #e1f0ff;
}
.steps-container {
  margin-bottom: 30px;
}
.step-cards {
  margin-bottom: 30px;
}
.step-card {
  cursor: pointer;
  transition: all 0.2s;
  height: 100%;
}
.step-card.active {
  border-color: #4F46E5;
  box-shadow: 0 4px 12px rgba(64, 158, 255, 0.15);
}
.step-card h3 {
  margin-top: 0;
  color: #303133;
}
.step-card .desc {
  color: #606266;
  font-size: 13px;
  margin: 8px 0;
}
.step-card ul {
  padding-left: 18px;
  color: #606266;
  font-size: 13px;
  line-height: 1.8;
}
.faq {
  margin-top: 20px;
}
.faq p {
  margin: 4px 0;
  line-height: 1.6;
}
</style>
