import { http } from '@/utils/request'

export const uploadDocument = (kbId, file, options = {}) => {
  const form = new FormData()
  form.append('file', file)
  form.append('type', options.type || detectType(file.name))
  if (options.metadata) form.append('metadata', JSON.stringify(options.metadata))
  return http.post(`/api/knowledge/${kbId}/upload`, form, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
};

export const importFromURL = (kbId, data) =>
  http.post(`/api/knowledge/${kbId}/import/url`, data)

export const importFromNotion = (kbId, data) =>
  http.post(`/api/knowledge/${kbId}/import/notion`, data)

export const importFromFeishu = (kbId, data) =>
  http.post(`/api/knowledge/${kbId}/import/feishu`, data)

export const importFromDingtalk = (kbId, data) =>
  http.post(`/api/knowledge/${kbId}/import/dingtalk`, data)

export const importFromCRM = (kbId, data) =>
  http.post(`/api/knowledge/${kbId}/import/crm`, data)

export const listDocumentTypes = () =>
  http.get('/api/knowledge/document-types')

function detectType(filename) {
  const ext = filename.split('.').pop().toLowerCase()
  return {
    md: 'markdown',
    markdown: 'markdown',
    pdf: 'pdf',
    doc: 'docx',
    docx: 'docx',
    html: 'html',
    htm: 'html',
    txt: 'text'
  }[ext] || 'text'
}
