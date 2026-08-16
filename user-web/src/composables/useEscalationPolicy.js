/**
 * 拟人度/置信度联动策略（USR-AI-06）
 * 当 AI 拟人度或置信度低于阈值时自动触发转人工
 */
import { ref, computed, watch } from 'vue'
import { http } from '@/utils/request'

// 全局配置（可从后端拉取）
const config = ref({
  humanizeThreshold: 0.7, // 拟人度阈值（低于此值转人工）
  confidenceThreshold: 0.6, // 置信度阈值
  autoTransferEnabled: true, // 是否自动转人工
  warnOnly: false // true = 仅警告不转人工
})

export function useEscalationPolicy() {
  const lastDecision = ref(null)

  /**
   * 评估单条 AI 回复
   * @param {object} aiResponse { content, confidence, humanize_score }
   * @returns {object} { shouldTransfer, reason, severity }
   */
  function evaluate(aiResponse) {
    if (!config.value.autoTransferEnabled) {
      return { shouldTransfer: false, reason: 'auto_disabled', severity: 'info' }
    }

    const { confidence = 1, humanize_score = 1 } = aiResponse

    if (humanize_score < config.value.humanizeThreshold) {
      return {
        shouldTransfer: !config.value.warnOnly,
        reason: 'low_humanize',
        severity: humanize_score < 0.4 ? 'critical' : 'warning',
        details: { humanize_score, threshold: config.value.humanizeThreshold }
      }
    }

    if (confidence < config.value.confidenceThreshold) {
      return {
        shouldTransfer: !config.value.warnOnly,
        reason: 'low_confidence',
        severity: confidence < 0.3 ? 'critical' : 'warning',
        details: { confidence, threshold: config.value.confidenceThreshold }
      }
    }

    return { shouldTransfer: false, reason: 'ok', severity: 'info' }
  }

  async function transferToHuman(sessionId, reason) {
    return http.post(`/api/customer-sessions/${sessionId}/transfer`, {
      reason,
      source: 'ai_escalation',
      auto: true
    })
  }

  async function loadConfig() {
    try {
      const res = await http.get('/api/admin/tuning/escalation-config', undefined, { _silent: true })
      if (res) Object.assign(config.value, res)
    } catch (_) {}
  }

  return { config, evaluate, transferToHuman, loadConfig, lastDecision }
}

export default { useEscalationPolicy }
