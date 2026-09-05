import { http } from '@/utils/request'

export const emailApi = {
  getEmailSmtpList() {
    return http.get('/api/email/smtp')
  },
  
  addEmailSmtp(data) {
    return http.post('/api/email/smtp', data)
  },
  
  updateEmailSmtp(id, data) {
    return http.put(`/api/email/smtp/${id}`, data)
  },
  
  deleteEmailSmtp(id) {
    return http.delete(`/api/email/smtp/${id}`)
  },
  
  getDrafts() {
    return http.get('/api/email/drafts')
  },
  
  getDraftDetail(id) {
    return http.get(`/api/email/drafts/${id}`)
  },
  
  createDraft(data) {
    return http.post('/api/email/drafts', data)
  },
  
  updateDraft(id, data) {
    return http.put(`/api/email/drafts/${id}`, data)
  },
  
  deleteDraft(id) {
    return http.delete(`/api/email/drafts/${id}`)
  },
  
  uploadImage(formData) {
    return http.post('/api/upload', formData, {
      headers: {
        'Content-Type': 'multipart/form-data'
      }
    })
  },
  
  sendEmail(data) {
    return http.post('/api/email/send', data)
  },
  getEmailList: (page,limit) => {
    return http.get(`/api/email/list?page=${page}&limit=${limit}`)
  },
  
  getJobsList(page, limit) {
    return http.get(`/api/email/jobs?page=${page}&limit=${limit}`)
  },
  deleteJob(id) {
    return http.delete(`/api/email/jobs/${id}`)
  },
  getJobDetail(id) {
    return http.get(`/api/email/jobs/${id}`)
  },

  getEmailTrace(id) {
    return http.get(`/api/email/lists/${id}/tracking`)
  },

  deleteEmailList(id) {
    return http.delete(`/api/email/list/${id}`)
  },
};