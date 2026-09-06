import { http } from '@/utils/request'

export const createReachJob = (data) => http.post('/api/reach/jobs', data);
export const getReachJob = (id) => http.get(`/api/reach/jobs/${id}`)
export const cancelReachJob = (id) => http.post(`/api/reach/jobs/${id}/cancel`, {})

export const createReachJobWithExperiment = (data) => {
  const { experiment_id, audience, channel, content, ...rest } = data
  return http.post('/api/reach/jobs/with-experiment', {
    experiment_id,
    channel,
    audience,
    content,
    split_method: experiment_id ? 'mab' : 'random',
    ...rest
  });
};

export const getExperimentResultsWithReach = (id) =>
  http.get(`/api/ab-experiments/${id}/results-with-reach`);

export const reportReachMetrics = (data) =>
  http.post('/api/ab-experiments/reach-metrics', data);
