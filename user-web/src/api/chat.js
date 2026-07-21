import http from '@/utils/request'

// 获取附件上传凭证（七牛直传 token）
export function getUploadToken(params) {
  return http.get('/api/chat/public/upload-token', { params })
}
