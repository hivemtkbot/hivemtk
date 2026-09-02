<template>
  <div class="custom-dashboard">
    <el-row :gutter="12" class="toolbar">
      <el-col :span="12">
        <span class="title">自定义看板拖拽（USR-AN-01）</span>
      </el-col>
      <el-col :span="12" style="text-align: right">
        <el-button @click="addBlock">+ 添加组件</el-button>
        <el-button type="primary" @click="save">保存看板</el-button>
      </el-col>
    </el-row>

    <el-row :gutter="12">
      <el-col :span="6">
        <el-card class="palette">
          <template #header><span>组件库</span></template>
          <div
            v-for="b in blockTypes"
            :key="b.type"
            class="palette-item"
            draggable="true"
            @dragstart="onPaletteDragStart($event, b)"
          >
            <el-icon><component :is="b.icon" /></el-icon>
            <span>{{ b.label }}</span>
          </div>
        </el-card>
      </el-col>
      <el-col :span="18">
        <div class="canvas" @drop="onDrop" @dragover.prevent>
          <div
            v-for="(block, i) in blocks"
            :key="block.id"
            class="canvas-block"
            :style="blockStyle(block)"
          >
            <div class="block-toolbar">
              <el-button-group size="small">
                <el-button @click="resize(i)">↔</el-button>
                <el-button type="danger" @click="removeBlock(i)">×</el-button>
              </el-button-group>
            </div>
            <component :is="resolveChart(block.type)" :data="block.data" />
            <div class="block-title">{{ block.title }}</div>
          </div>
        </div>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
/**
 * 自定义看板拖拽（USR-AN-01）
 * 借鉴：Metabase / Apache Superset
 */
import { ref, reactive, markRaw } from 'vue'
import { ElMessage } from 'element-plus'
import {
  DataLine, PieChart, Document, Histogram, TrendCharts, LocationInformation
} from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { safeInit } from '@/utils/echarts'
import { http } from '@/utils/request'

const blockTypes = [
  { type: 'line', label: '折线', icon: markRaw(TrendCharts) },
  { type: 'bar', label: '柱状', icon: markRaw(DataLine) },
  { type: 'pie', label: '饼图', icon: markRaw(PieChart) },
  { type: 'kpi', label: '数字卡', icon: markRaw(Document) },
  { type: 'funnel', label: '漏斗', icon: markRaw(Histogram) },
  { type: 'map', label: '地图', icon: markRaw(LocationInformation) }
]

const blocks = ref([])
let _id = 1

function onPaletteDragStart(e, b) {
  e.dataTransfer.setData('blockType', b.type)
}

function onDrop(e) {
  const type = e.dataTransfer.getData('blockType')
  if (!type) return
  blocks.value.push({
    id: _id++,
    type,
    title: blockTypes.find((b) => b.type === type).label,
    width: 6,
    height: 240,
    data: { value: Math.floor(Math.random() * 1000) }
  })
  ElMessage.success('已添加组件')
}

function resolveChart(type) {
  return markRaw({
    line: defineAsyncComponent(() => loadChart(type)),
    bar: defineAsyncComponent(() => loadChart(type)),
    pie: defineAsyncComponent(() => loadChart(type)),
    kpi: markRaw({ template: '<div class="kpi">{{ data.value }}</div>' }),
    funnel: defineAsyncComponent(() => loadChart(type)),
    map: defineAsyncComponent(() => loadChart(type))
  }[type] || markRaw({ template: '未知' }))
}

import { defineAsyncComponent } from 'vue'
function loadChart(type) {
  return {
    template: `<div ref="chart" class="chart-area"></div>`,
    mounted() {
      const c = safeInit(this.$refs.chart)
      c.setOption({
        xAxis: { type: 'category', data: ['A', 'B', 'C', 'D'] },
        yAxis: { type: 'value' },
        series: [{ type: '${type}', data: [10, 20, 30, 40] }]
      })
    }
  }
}

function blockStyle(b) {
  return {
    width: `${(b.width / 12) * 100}%`,
    height: `${b.height}px`
  }
}

function addBlock() { /* 同 onDrop */ }
function resize(i) {
  blocks.value[i].width = blocks.value[i].width === 6 ? 12 : 6
}
function removeBlock(i) { blocks.value.splice(i, 1) }

async function save() {
  await http.post('/api/dashboards', { blocks: blocks.value })
  ElMessage.success('看板已保存')
}
</script>

<style scoped>
.custom-dashboard { padding: 16px; }
.toolbar { margin-bottom: 16px; }
.title { font-size: 16px; font-weight: 600; }
.palette-item { padding: 8px; margin-bottom: 4px; background: #F8FAFC; border-radius: 4px; cursor: grab; display: flex; align-items: center; gap: 6px; }
.canvas { min-height: 600px; background: #fff; border-radius: 8px; padding: 16px; }
.canvas-block { position: relative; display: inline-block; background: #F8FAFC; border-radius: 6px; padding: 12px; margin: 4px; vertical-align: top; }
.block-toolbar { position: absolute; right: 4px; top: 4px; }
.chart-area { width: 100%; height: 180px; }
.block-title { text-align: center; font-size: 12px; color: #94A3B8; margin-top: 4px; }
.kpi { font-size: 32px; font-weight: 700; text-align: center; color: #4F46E5; }
</style>
