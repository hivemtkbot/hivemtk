import { http } from '@/utils/request'

export const clueApi = {
  delete: (id) => {
    return http.delete(`/api/clues/delete/${id}`)
  },
  list: (page,limit) => {
    return http.get(`/api/clues/list?page=${page}&limit=${limit}`)
  },
  statistics: () => {
    return http.get(`/api/clues/statistics`)
  },
  import: (data) => {
    return http.post(`/api/clues/import`,data)
  },
}