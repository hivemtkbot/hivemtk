<template>
  <div class="kb-citation">
    <el-card v-for="(cite, i) in citations" :key="i" class="cite-card" @click="openOriginal(cite)">
      <div class="cite-header">
        <el-tag size="small" :type="i === 0 ? 'primary' : 'info'">#{{ i + 1 }}</el-tag>
        <span class="cite-title">{{ cite.title || cite.document_title }}</span>
        <el-tag size="small" :type="scoreTag(cite.score)">相关度 {{ (cite.score * 100).toFixed(0) }}%</el-tag>
      </div>
      <div class="cite-content" v-html="highlightedContent(cite)" />
      <div class="cite-meta">
        <span v-if="cite.source_path">📄 {{ cite.source_path }}:{{ cite.line || '?' }}</span>
        <span v-if="cite.page">· 第 {{ cite.page }} 页</span>
        <el-button-group size="small" style="margin-left: auto">
          <el-button :icon="Pointer" @click.stop="copyContent(cite)">复制</el-button>
          <el-button :icon="View" @click.stop="openOriginal(cite)">查看原文</el-button>
        </el-button-group>
      </div>
      <div class="cite-feedback">
        <el-button-group size="small">
          <el-button :type="cite.feedback === 'up' ? 'success' : ''" @click.stop="vote(cite, 'up')">
            <el-icon><CaretTop /></el-icon>
            有帮助 ({{ cite.upCount || 0 }})
          </el-button>
          <el-button :type="cite.feedback === 'down' ? 'danger' : ''" @click.stop="vote(cite, 'down')">
            <el-icon><CaretBottom /></el-icon>
            无帮助 ({{ cite.downCount || 0 }})
          </el-button>
        </el-button-group>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue';
import { ElMessage } from 'element-plus'
import { Pointer, View, CaretTop, CaretBottom } from '@element-plus/icons-vue'
import { http } from '@/utils/request'

const props = defineProps({
  citations: { type: Array, default: () => [] },
  query: { type: String, default: '' }
})
const emit = defineEmits(['feedback'])

function scoreTag(score) {
  if (score >= 0.8) return 'success'
  if (score >= 0.6) return 'primary'
  if (score >= 0.4) return 'warning'
  return 'info'
}

function highlightedContent(cite) {
  let content = cite.content || ''
  if (!props.query) return content
  const tokens = props.query.split(/\s+/).filter((t) => t.length > 1);
  tokens.forEach((t) => {
    const re = new RegExp(`(${escape(t)})`, 'gi')
    content = content.replace(re, '<mark>$1</mark>')
  })
  return content
}

function escape(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function copyContent(cite) {
  navigator.clipboard?.writeText(cite.content || '')
  ElMessage.success('已复制')
}

function openOriginal(cite) {
  if (cite.source_url) {
    window.open(cite.source_url, '_blank')
  } else if (cite.document_id) {
    window.open(`/knowledge/management?doc=${cite.document_id}`, '_blank')
  }
}

async function vote(cite, type) {
  cite.feedback = cite.feedback === type ? null : type
  cite.upCount = (cite.upCount || 0) + (type === 'up' ? 1 : 0) - (cite.feedback === 'up' ? 1 : 0)
  cite.downCount = (cite.downCount || 0) + (type === 'down' ? 1 : 0) - (cite.feedback === 'down' ? 1 : 0)
  await http.post(`/api/knowledge/citations/${cite.id}/feedback`, { type })
  emit('feedback', { citation: cite, type })
}
</script>

<style scoped>
.kb-citation { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
.cite-card {
  cursor: pointer;
  transition: all 0.2s ease;
  border-left: 3px solid #6366F1;
}
.cite-card:hover { border-color: #4F46E5; box-shadow: 0 2px 8px rgba(99, 102, 241, 0.15); }
.cite-header { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.cite-title { font-weight: 600; color: #0F172A; flex: 1; }
.cite-content { font-size: 13px; color: #475569; line-height: 1.6; max-height: 100px; overflow: hidden; }
.cite-content :deep(mark) { background: #FEF08A; color: #0F172A; padding: 0 2px; border-radius: 2px; }
.cite-meta { display: flex; align-items: center; gap: 6px; font-size: 12px; color: #94A3B8; margin-top: 6px; }
.cite-feedback { margin-top: 6px; }
</style>
