import { http } from '@/utils/request'

/**
 * 触达 → A/B 实验打通（USR-RC-03）
 * 发送任务可绑定 experiment_id，效果回流到实验
 */

// 触达任务 API
export const createReachJob = (data) => http.post('/api/reach/jobs', data)
export const getReachJob = (id) => http.get(`/api/reach/jobs/${id}`)
export const cancelReachJob = (id) => http.post(`/api/reach/jobs/${id}/cancel`, {})

// 创建绑定实验的触达任务
export const createReachJobWithExperiment = (data) => {
  const { experiment_id, audience, channel, content, ...rest } = data
  return http.post('/api/reach/jobs/with-experiment', {
    experiment_id,
    channel,
    audience,
    content,
    split_method: experiment_id ? 'mab' : 'random', // 绑实验用 MAB
    ...rest
  })
}

// 实验效果聚合（带触达渠道维度）
export const getExperimentResultsWithReach = (id) =>
  http.get(`/api/ab-experiments/${id}/results-with-reach`)

// 触达效果回流入实验
export const reportReachMetrics = (data) =>
  http.post('/api/ab-experiments/reach-metrics', data)
