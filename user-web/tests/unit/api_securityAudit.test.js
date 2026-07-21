/**
 * 安全审计 API 单元测试
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/utils/request', () => ({ default: request }))

import {
  runSecurityAudit,
  getSecurityAuditList,
  getSecurityAuditDetail
} from '@/api/securityAudit.js'

describe('安全审计 API', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockResolvedValue({ code: 'SUCCESS', data: {} })
  })

  it('runSecurityAudit 提交审计名', async () => {
    await runSecurityAudit({ audit_name: 'manual_001' })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: '/api/security/audit',
        method: 'post',
        data: { audit_name: 'manual_001' }
      })
    )
  })

  it('getSecurityAuditList 接受分页参数', async () => {
    await getSecurityAuditList({ page: 1, page_size: 20 })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: '/api/security/audit/list',
        method: 'get',
        params: { page: 1, page_size: 20 }
      })
    )
  })

  it('getSecurityAuditDetail 拼接 ID', async () => {
    await getSecurityAuditDetail(42)
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/security/audit/42', method: 'get' })
    )
  })
})
