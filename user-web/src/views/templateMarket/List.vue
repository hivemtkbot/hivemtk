<template>
  <div class="template-market-page">
    <el-card class="header-card">
      <div>
        <h2>{{ $t('模板市场') }}</h2>
        <p class="subtitle">{{ $t('丰富的营销模板，开箱即用') }}</p>
      </div>
      <el-button type="primary" @click="showSubmitDialog">
        <el-icon><Upload /></el-icon>
        {{ $t('提交模板') }}
      </el-button>
    </el-card>

    <div class="filter-bar">
      <el-input v-model="searchKeyword" :placeholder="$t('搜索模板')" clearable style="width: 250px" />
      <el-select v-model="filterCategory" :placeholder="$t('分类')" clearable style="width: 150px">
        <el-option :label="$t('邮件模板')" value="email" />
        <el-option :label="$t('短信模板')" value="sms" />
        <el-option label="WhatsApp" value="whatsapp" />
        <el-option :label="$t('活动落地页')" value="landing" />
        <el-option :label="$t('海报')" value="poster" />
        <el-option :label="$t('话术')" value="script" />
      </el-select>
      <el-select v-model="filterTag" :placeholder="$t('标签')" clearable style="width: 150px">
        <el-option :label="$t('热门')" value="hot" />
        <el-option :label="$t('新品')" value="new" />
        <el-option :label="$t('免费')" value="free" />
        <el-option :label="$t('付费')" value="paid" />
      </el-select>
      <el-radio-group v-model="sortBy">
        <el-radio-button label="latest">{{ $t('最新') }}</el-radio-button>
        <el-radio-button label="popular">{{ $t('最热') }}</el-radio-button>
        <el-radio-button label="rating">{{ $t('评分') }}</el-radio-button>
      </el-radio-group>
    </div>

    <el-row :gutter="20" class="templates-grid">
      <el-col :span="6" v-for="template in filteredTemplates" :key="template.id">
        <el-card class="template-card" shadow="hover" @click="viewTemplate(template)">
          <div class="template-cover">
            <img :src="template.cover" :alt="template.name" v-if="template.cover" />
            <div v-else class="cover-placeholder">{{ template.category }}</div>
            <el-tag v-if="template.isNew" type="danger" size="small" class="tag-new">NEW</el-tag>
            <el-tag v-if="template.isHot" type="warning" size="small" class="tag-hot">HOT</el-tag>
          </div>
          <div class="template-info">
            <h4>{{ template.name }}</h4>
            <p class="desc">{{ template.description }}</p>
            <div class="meta">
              <el-rate v-model="template.rating" disabled show-score size="small" />
              <span class="downloads">⬇ {{ template.downloads }}</span>
            </div>
            <div class="footer">
              <el-tag size="small">{{ template.category }}</el-tag>
              <span class="price" v-if="template.price > 0">¥{{ (template.price / 100).toFixed(2) }}</span>
              <span class="price free" v-else>{{ $t('免费') }}</span>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-dialog v-model="detailVisible" :title="currentTemplate?.name" width="800px">
      <div v-if="currentTemplate" class="template-detail">
        <el-carousel v-if="currentTemplate.screenshots" height="300px">
          <el-carousel-item v-for="(img, idx) in currentTemplate.screenshots" :key="idx">
            <img :src="img" style="width: 100%; height: 100%; object-fit: contain" />
          </el-carousel-item>
        </el-carousel>
        <el-descriptions :column="2" border style="margin-top: 20px">
          <el-descriptions-item :label="$t('分类')">{{ currentTemplate.category }}</el-descriptions-item>
          <el-descriptions-item :label="$t('价格')">
            <span v-if="currentTemplate.price > 0" style="color: #EF4444">¥{{ currentTemplate.price }}</span>
            <span v-else style="color: #10B981">{{ $t('免费') }}</span>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('评分')">
            <el-rate v-model="currentTemplate.rating" disabled />
          </el-descriptions-item>
          <el-descriptions-item :label="$t('下载量')">{{ currentTemplate.downloads }}</el-descriptions-item>
          <el-descriptions-item :label="$t('作者')">{{ currentTemplate.author }}</el-descriptions-item>
          <el-descriptions-item :label="$t('更新日期')">{{ currentTemplate.updatedAt }}</el-descriptions-item>
        </el-descriptions>
        <div class="description">
          <h4>{{ $t('模板介绍') }}</h4>
          <p>{{ currentTemplate.description }}</p>
        </div>
        <div class="preview">
          <h4>{{ $t('内容预览') }}</h4>
          <pre>{{ currentTemplate.preview }}</pre>
        </div>
      </div>
      <template #footer>
        <el-button @click="detailVisible = false">{{ $t('关闭') }}</el-button>
        <el-button type="primary" @click="useTemplate">
          <el-icon><Download /></el-icon>
          {{ $t('立即使用') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="submitVisible" title="提交模板" width="600px">
      <el-form :model="submitForm" label-width="100px">
        <el-form-item label="模板名称">
          <el-input v-model="submitForm.name" />
        </el-form-item>
        <el-form-item label="分类">
          <el-select v-model="submitForm.category" style="width: 100%">
            <el-option label="邮件模板" value="email" />
            <el-option label="短信模板" value="sms" />
            <el-option label="WhatsApp" value="whatsapp" />
            <el-option label="活动落地页" value="landing" />
            <el-option label="海报" value="poster" />
            <el-option label="话术" value="script" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="submitForm.description" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item label="模板内容">
          <el-input v-model="submitForm.content" type="textarea" :rows="6" />
        </el-form-item>
        <el-form-item label="封面图">
          <el-input v-model="submitForm.cover" placeholder="图片URL" />
        </el-form-item>
        <el-form-item label="标签">
          <el-input v-model="submitForm.tags" placeholder="多个标签用逗号分隔" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="submitVisible = false">取消</el-button>
        <el-button type="primary" @click="submitTemplate">提交</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import i18n from '@/i18n'

import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Upload, Download } from '@element-plus/icons-vue'
import { getTemplates, submitTemplate, useTemplate as useTpl } from '@/api/templateMarket.js'

const searchKeyword = ref('')
const filterCategory = ref('')
const filterTag = ref('')
const sortBy = ref('latest')
const templates = ref([])
const detailVisible = ref(false)
const currentTemplate = ref(null)
const submitVisible = ref(false)
const submitForm = ref({ name: '', category: 'email', description: '', content: '', cover: '', tags: '' })

const filteredTemplates = computed(() => {
  let result = templates.value
  if (searchKeyword.value) result = result.filter(t => t.name.includes(searchKeyword.value))
  if (filterCategory.value) result = result.filter(t => t.category === filterCategory.value)
  return result
})

const loadTemplates = async () => {
  const res = await getTemplates({ sort: sortBy.value })
  templates.value = res.data || []
}

const viewTemplate = (template) => {
  currentTemplate.value = template
  detailVisible.value = true
}

const useTemplate = async () => {
  await useTpl(currentTemplate.value.id)
  ElMessage.success(i18n.global.t('模板已添加到我的模板'))
  detailVisible.value = false
}

const showSubmitDialog = () => {
  submitForm.value = { name: '', category: 'email', description: '', content: '', cover: '', tags: '' }
  submitVisible.value = true
}

const submitTemplateForm = async () => {
  await submitTemplate(submitForm.value)
  ElMessage.success(i18n.global.t('模板已提交，等待审核'))
  submitVisible.value = false
  loadTemplates()
}

onMounted(() => loadTemplates())
</script>

<style scoped lang="scss">
.template-market-page { padding: 20px; }
.header-card {
  margin-bottom: 20px;
  :deep(.el-card__body) { display: flex; justify-content: space-between; align-items: center; }
  h2 { margin: 0 0 8px 0; }
  .subtitle { color: #909399; margin: 0; }
}
.filter-bar {
  display: flex;
  gap: 15px;
  margin-bottom: 20px;
  align-items: center;
}
.templates-grid {
  .template-card {
    cursor: pointer;
    transition: transform 0.3s;
    &:hover { transform: translateY(-5px); }
    :deep(.el-card__body) { padding: 0; }
  }
  .template-cover {
    height: 180px;
    background: #f5f7fa;
    position: relative;
    overflow: hidden;
    img { width: 100%; height: 100%; object-fit: cover; }
    .cover-placeholder {
      display: flex;
      align-items: center;
      justify-content: center;
      height: 100%;
      color: #909399;
      font-size: 18px;
    }
    .tag-new { position: absolute; top: 10px; left: 10px; }
    .tag-hot { position: absolute; top: 10px; right: 10px; }
  }
  .template-info {
    padding: 15px;
    h4 { margin: 0 0 8px 0; }
    .desc {
      color: #909399;
      font-size: 13px;
      height: 36px;
      overflow: hidden;
      text-overflow: ellipsis;
      display: -webkit-box;
      -webkit-line-clamp: 2;
      -webkit-box-orient: vertical;
    }
    .meta {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin: 10px 0;
      .downloads { color: #909399; font-size: 12px; }
    }
    .footer {
      display: flex;
      justify-content: space-between;
      align-items: center;
      .price { color: #EF4444; font-weight: bold; &.free { color: #10B981; } }
    }
  }
}
.template-detail {
  .description, .preview {
    margin-top: 20px;
    h4 { margin-bottom: 10px; }
    pre {
      background: #f5f7fa;
      padding: 15px;
      border-radius: 4px;
      white-space: pre-wrap;
    }
  }
}
</style>
