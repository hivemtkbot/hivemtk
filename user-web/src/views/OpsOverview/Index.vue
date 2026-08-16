<template>
  <div class="ops-overview">
    <PageHeader title="运维总览" subtitle="系统健康度、关键指标、告警一览" />

    <!-- 顶部状态卡片 -->
    <el-row :gutter="20" class="status-row">
      <el-col :span="6">
        <el-card class="status-card status-ok">
          <div class="status-label">系统状态</div>
          <div class="status-value">{{ systemStatus.text }}</div>
          <div class="status-meta">运行时长 {{ uptime }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="status-card">
          <div class="status-label">在线用户</div>
          <div class="status-value">{{ stats.onlineUsers }}</div>
          <div class="status-meta">总用户 {{ stats.totalUsers }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="status-card">
          <div class="status-label">今日消息</div>
          <div class="status-value">{{ stats.todayMessages }}</div>
          <div class="status-meta">环比 {{ stats.messagesDelta }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card :class="['status-card', alertsCount > 0 ? 'status-warn' : '']">
          <div class="status-label">未处理告警</div>
          <div class="status-value">{{ alertsCount }}</div>
          <div class="status-meta">查看告警中心</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 二级：模块状态 -->
    <el-row :gutter="20" class="mt-20">
      <el-col :span="12">
        <el-card header="11 大系统模块状态">
          <el-table :data="moduleStatus" stripe>
            <el-table-column prop="name" label="模块" />
            <el-table-column prop="status" label="状态">
              <template #default="{ row }">
                <el-tag :type="row.status === 'ok' ? 'success' : row.status === 'warn' ? 'warning' : 'danger'">
                  {{ row.status === 'ok' ? '正常' : row.status === 'warn' ? '告警' : '异常' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="latency" label="延迟 (ms)" />
            <el-table-column prop="qps" label="QPS" />
            <el-table-column prop="errorRate" label="错误率" />
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card header="最近 24h 关键操作">
          <el-table :data="recentOps" stripe max-height="400">
            <el-table-column prop="time" label="时间" width="100" />
            <el-table-column prop="user" label="操作人" />
            <el-table-column prop="action" label="操作" />
            <el-table-column prop="result" label="结果">
              <template #default="{ row }">
                <el-tag :type="row.result === 'success' ? 'success' : 'danger'" size="small">
                  {{ row.result === 'success' ? '成功' : '失败' }}
                </el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import PageHeader from '@/components/PageHeader.vue'
import { getSystemStatus, getOpsStats, getModuleStatus, getRecentOperations } from '@/api/cockpit'

const systemStatus = ref({ text: '加载中...', code: 'loading' })
const uptime = ref('0h 0m')
const stats = ref({ onlineUsers: 0, totalUsers: 0, todayMessages: 0, messagesDelta: '0%' })
const moduleStatus = ref([])
const recentOps = ref([])
const alertsCount = ref(0)

async function loadOverview() {
  try {
    const [status, opsStats, modules, ops, alerts] = await Promise.all([
      getSystemStatus(),
      getOpsStats(),
      getModuleStatus(),
      getRecentOperations(20),
      fetch('/api/monitor/alerts/unread').then(r => r.json())
    ])
    systemStatus.value = status.data
    uptime.value = status.data.uptime
    stats.value = opsStats.data
    moduleStatus.value = modules.data
    recentOps.value = ops.data
    alertsCount.value = alerts.data?.count || 0
  } catch (err) {
    console.error('load overview failed:', err)
  }
}

onMounted(() => {
  loadOverview()
  setInterval(loadOverview, 30000) // 30s 刷新
})
</script>

<style scoped>
.ops-overview {
  padding: 20px;
}
.status-row { margin-top: 16px; }
.status-card {
  text-align: center;
  padding: 12px 0;
}
.status-card .status-label { font-size: 14px; color: #909399; }
.status-card .status-value { font-size: 32px; font-weight: 600; color: #303133; margin: 12px 0; }
.status-card .status-meta { font-size: 12px; color: #909399; }
.status-ok { border-left: 4px solid #67c23a; }
.status-warn { border-left: 4px solid #e6a23c; }
.mt-20 { margin-top: 20px; }
</style>
