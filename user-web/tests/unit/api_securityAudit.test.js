/**
 * 安全审计 API 单元测试
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

const { request, http } = vi.hoisted(() => {
  const request = vi.fn()
  const http = {
    get: vi.fn(),
    post: vi.fn(),
    put: vi.fn(),
    delete: vi.fn()
  }
  return { request, http }
})
vi.mock('@/utils/request', () => ({ default: request, http }))

import {
  runSecurityAudit,
  getSecurityAuditList,
  getSecurityAuditDetail
} from '@/api/securityAudit.js'

describe('安全审计 API', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockResolvedValue({ code: 'SUCCESS', data: {} })
    http.get.mockReset()
    http.get.mockResolvedValue({ code: 'SUCCESS', data: {} })
  })

  it('runSecurityAudit 提交审计名', async () => {
    await runSecurityAudit({ audit_name: 'manual_001' })
    expect(http.post).toHaveBeenCalledWith('/api/security/audit', { audit_name: 'manual_001' })
  })

  it('getSecurityAuditList 接受分页参数', async () => {
    await getSecurityAuditList({ page: 1, page_size: 20 })
    expect(http.get).toHaveBeenCalledWith('/api/security/audit/list', { page: 1, page_size: 20 })
  })

  it('getSecurityAuditDetail 拼接 ID', async () => {
    await getSecurityAuditDetail(42)
    expect(http.get).toHaveBeenCalledWith('/api/security/audit/42')
  })
})
