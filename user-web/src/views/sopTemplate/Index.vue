<template>
  <div class="sop-template-index-page">
    <el-card class="header-card" shadow="never">
      <div class="header-content">
        <div>
          <h2>SOP 模板库</h2>
          <p class="subtitle">Layer1 SOP 模板匹配 · 命中即跳过 LLM 直接渲染模板变量</p>
        </div>
      </div>
    </el-card>

    <el-row :gutter="20">
      <el-col :span="8">
        <el-card shadow="hover" class="entry-card" @click="$router.push('/sop-template/list')">
          <div class="entry-icon"><el-icon><Tickets /></el-icon></div>
          <div class="entry-title">SOP 模板列表</div>
          <div class="entry-desc">管理 (intent, stage) 维度的回复模板，支持 Go 模板变量渲染</div>
          <el-button type="primary" plain>
            进入
            <el-icon class="el-icon--right"><ArrowRight /></el-icon>
          </el-button>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="hover" class="entry-card">
          <div class="entry-icon"><el-icon><Connection /></el-icon></div>
          <div class="entry-title">匹配测试</div>
          <div class="entry-desc">在 SOP 模板列表页可对单条模板执行 Layer1 (intent, stage) 匹配验证</div>
          <el-button type="primary" plain @click="$router.push('/sop-template/list')">
            打开
            <el-icon class="el-icon--right"><ArrowRight /></el-icon>
          </el-button>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="hover" class="entry-card">
          <div class="entry-icon"><el-icon><Cpu /></el-icon></div>
          <div class="entry-title">智能体绑定</div>
          <div class="entry-desc">在 AI 智能体编辑页 → 知识库绑定 → 选择要挂载的 SOP 模板范围（可多选）</div>
          <el-button type="primary" plain @click="$router.push('/aiAgent/list')">
            去绑定
            <el-icon class="el-icon--right"><ArrowRight /></el-icon>
          </el-button>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { Tickets, Cpu, Connection, ArrowRight } from '@element-plus/icons-vue'
</script>

<style scoped>
.sop-template-index-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.header-card :deep(.el-card__body) {
  padding: 18px 24px;
}

.header-content h2 {
  margin: 0 0 4px 0;
  font-size: 20px;
  font-weight: 600;
}

.subtitle {
  margin: 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.entry-card {
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.2s;
  text-align: center;
  padding: 8px 0;
}

.entry-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
}

.entry-icon {
  font-size: 36px;
  color: var(--el-color-primary);
  margin-bottom: 8px;
}

.entry-title {
  font-size: 16px;
  font-weight: 600;
  margin-bottom: 6px;
}

.entry-desc {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.5;
  margin-bottom: 12px;
  min-height: 40px;
}
</style>
