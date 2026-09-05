import { ref, computed, watch } from 'vue';
import { http } from '@/utils/request'

const config = ref({
  humanizeThreshold: 0.7,
  confidenceThreshold: 0.6,
  autoTransferEnabled: true,
  warnOnly: false
});

export function useEscalationPolicy() {
  const lastDecision = ref(null)

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
