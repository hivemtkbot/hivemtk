import { http } from '@/utils/request';

// 话术版本管理 + AB 曝光统计（R55 T5，对接后端 /api/script-library/* /api/script-ab/*）
export const scriptAbApi = {
  // 版本列表
  listVersions: (scriptId) => http.get(`/api/script-library/${scriptId}/versions`),
  // 创建版本快照
  createVersion: (scriptId, data) => http.post(`/api/script-library/${scriptId}/versions`, data),
  // 激活指定版本
  activateVersion: (scriptId, versionId) =>
    http.put(`/api/script-library/${scriptId}/versions/${versionId}/activate`),
  // 废弃话术
  expireScript: (scriptId, data) => http.post(`/api/script-library/${scriptId}/expire`, data),
  // AB 曝光/转化统计
  getAbStats: (scriptId, params) => http.get(`/api/script-library/${scriptId}/ab-stats`, params),
  // 更新 AB 配置（split_a/attribution_h/enabled）
  updateAbConfig: (scriptId, data) => http.put(`/api/script-library/${scriptId}/ab-config`, data),
  // 手动回写转化
  recordConversion: (data) => http.post('/api/script-ab/conversion', data),
};
