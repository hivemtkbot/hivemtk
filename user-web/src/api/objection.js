import { http } from '@/utils/request';

export function handleObjection(data) {
  return http.post('/api/objection/handle', data)
}

export function classifyObjection(data) {
  return http.post('/api/objection/classify', data)
}

export function listObjectionCategories() {
  return http.get('/api/objection/categories')
}

export function recordObjectionUsage(data) {
  return http.post('/api/objection/usage', data)
}
