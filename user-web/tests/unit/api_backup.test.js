/**
 * 备份 API 单元测试
 * 验证备份/恢复 API 函数调用入参与返回处理
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

// hoisted 后可在 vi.mock 工厂中访问
const { request } = vi.hoisted(() => ({ request: vi.fn() }))
vi.mock('@/utils/request', () => ({ default: request }))

import {
  getBackupList,
  getBackupByID,
  createBackup,
  deleteBackup,
  restoreBackup,
  getRestoreList,
  getLastRestore
} from '@/api/backup.js'

describe('备份 API', () => {
  beforeEach(() => {
    request.mockReset()
    request.mockResolvedValue({ code: 'SUCCESS', data: {} })
  })

  it('getBackupList 传入分页参数', async () => {
    await getBackupList({ page: 2, page_size: 50 })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: '/api/backups',
        method: 'get',
        params: { page: 2, page_size: 50 }
      })
    )
  })

  it('getBackupByID 拼接 ID', async () => {
    await getBackupByID(7)
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/backups/7', method: 'get' })
    )
  })

  it('createBackup 提交 JSON body', async () => {
    await createBackup({ backup_name: 'b1', backup_type: 'full' })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: '/api/backups',
        method: 'post',
        data: { backup_name: 'b1', backup_type: 'full' }
      })
    )
  })

  it('deleteBackup 调用 DELETE', async () => {
    await deleteBackup(99)
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/backups/99', method: 'delete' })
    )
  })

  it('restoreBackup 提交 backup_id', async () => {
    await restoreBackup({ backup_id: 5 })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({
        url: '/api/restore',
        method: 'post',
        data: { backup_id: 5 }
      })
    )
  })

  it('getRestoreList 接受分页参数', async () => {
    await getRestoreList({ page: 1, page_size: 20 })
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/restore/list', method: 'get' })
    )
  })

  it('getLastRestore 路径正确', async () => {
    await getLastRestore()
    expect(request).toHaveBeenCalledWith(
      expect.objectContaining({ url: '/api/restore/last', method: 'get' })
    )
  })
})
