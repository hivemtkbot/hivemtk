import { http } from '@/utils/request'

export function listAccounts(params) { return http.get('/api/whatsapp/accounts', { params }) }
export const getAccounts = listAccounts;
export function createAccount(data) { return http.post('/api/whatsapp/accounts', data) }
export function startLogin(id) { return http.post(`/api/whatsapp/accounts/${id}/login/start`) }
export function loginStatus(id) { return http.get(`/api/whatsapp/accounts/${id}/login/status`) }
export function updateAccount(id, data) { return http.put(`/api/whatsapp/accounts/${id}`, data) }
export function deleteAccount(id) { return http.delete(`/api/whatsapp/accounts/${id}`) }

export function listDrafts(params) { return http.get('/api/whatsapp/drafts', { params }) }
export function createDraft(data) { return http.post('/api/whatsapp/drafts', data) }
export function updateDraft(id, data) { return http.put(`/api/whatsapp/drafts/${id}`, data) }
export function deleteDraft(id) { return http.delete(`/api/whatsapp/drafts/${id}`) }

export function listJobs(params) { return http.get('/api/whatsapp/jobs', { params }) }
export function createJob(data) { return http.post('/api/whatsapp/jobs', data) }
export function getJob(id) { return http.get(`/api/whatsapp/jobs/${id}`) }
export function deleteJob(id) { return http.delete(`/api/whatsapp/jobs/${id}`) }

export default {
  listAccounts,
  getAccounts,
  createAccount,
  startLogin,
  loginStatus,
  updateAccount,
  deleteAccount,
  listDrafts,
  createDraft,
  updateDraft,
  deleteDraft,
  listJobs,
  createJob,
  getJob,
  deleteJob,
}
