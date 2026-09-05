<template>
  <div class="playground-advanced">
    <el-card>
      <template #header>
        <span>高级调参（USR-KB-04）</span>
      </template>
      <el-form :model="params" label-width="160px">
        <el-form-item label="Top K">
          <el-slider v-model="params.topK" :min="1" :max="50" :step="1" show-input show-stops />
          <div class="hint">向量检索的候选数（默认 5，建议 5-20）</div>
        </el-form-item>
        <el-form-item label="相似度阈值">
          <el-slider v-model="params.similarityThreshold" :min="0" :max="1" :step="0.05" show-input />
          <div class="hint">低于此分数的 chunk 被过滤（默认 0.5）</div>
        </el-form-item>
        <el-form-item label="向量权重">
          <el-slider v-model="params.vectorWeight" :min="0" :max="1" :step="0.1" show-input />
          <div class="hint">Hybrid 模式下，向量 vs 关键词的权重</div>
        </el-form-item>
        <el-form-item label="关键词权重">
          <el-slider :model-value="1 - params.vectorWeight" :min="0" :max="1" :step="0.1" disabled />
          <div class="hint">自动 = 1 - 向量权重</div>
        </el-form-item>
        <el-form-item label="启用 Rerank">
          <el-switch v-model="params.rerankEnabled" />
          <div class="hint">Rerank 模型二次精排（推荐开启）</div>
        </el-form-item>
        <el-form-item label="Rerank Top N">
          <el-input-number v-model="params.rerankTopN" :min="1" :max="100" :disabled="!params.rerankEnabled" />
        </el-form-item>
        <el-form-item label="过滤条件">
          <el-input v-model="params.filtersText" type="textarea" :rows="2" placeholder='例: {"category": "FAQ", "lang": "zh"}' />
        </el-form-item>
        <el-form-item>
          <el-button @click="saveAsPreset">保存为预设</el-button>
          <el-button type="primary" @click="apply">应用并重新检索</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, defineEmits } from 'vue';
import { ElMessage } from 'element-plus'
import { http } from '@/utils/request'

const props = defineProps({
  modelValue: { type: Object, default: () => ({}) }
})
const emit = defineEmits(['update:modelValue', 'apply'])

const params = reactive({
  topK: props.modelValue.topK || 5,
  similarityThreshold: props.modelValue.similarityThreshold || 0.5,
  vectorWeight: props.modelValue.vectorWeight ?? 0.7,
  rerankEnabled: props.modelValue.rerankEnabled ?? true,
  rerankTopN: props.modelValue.rerankTopN || 5,
  filtersText: props.modelValue.filtersText || ''
})

function apply() {
  let filters = {}
  if (params.filtersText.trim()) {
    try { filters = JSON.parse(params.filtersText) } catch (e) {
      ElMessage.error('过滤条件 JSON 格式错误')
      return
    }
  }
  emit('update:modelValue', { ...params, filters })
  emit('apply', { ...params, filters })
  ElMessage.success('参数已应用')
}

async function saveAsPreset() {
  const name = prompt('预设名称')
  if (!name) return
  await http.post('/api/knowledge/playground/presets', { name, params })
  ElMessage.success('预设已保存')
}
</script>

<style scoped>
.playground-advanced { padding: 16px; }
.hint { font-size: 11px; color: #94A3B8; margin-top: 2px; }
</style>
