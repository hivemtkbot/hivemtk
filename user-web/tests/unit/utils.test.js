/**
 * 工具函数单元测试
 * 覆盖 initHelper / map 等关键工具
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'

// 模拟 localStorage
const store = {}
globalThis.localStorage = {
  setItem: vi.fn((k, v) => { store[k] = v }),
  getItem: vi.fn((k) => store[k] ?? null),
  removeItem: vi.fn((k) => { delete store[k] }),
  clear: vi.fn(() => { Object.keys(store).forEach(k => delete store[k]) })
}

import {
  isInitialized,
  markInitializationComplete,
  INIT_STATUS_KEY
} from '@/utils/initHelper.js'
import {
  getPlatformTag,
  getPlatformName,
  getPlatformMap,
  getStatusTag,
  getStatusName,
  getClueMap,
  getClueName
} from '@/utils/map.js'

describe('initHelper', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('初始状态未初始化', () => {
    expect(isInitialized()).toBe(false)
  })

  it('markInitializationComplete 后变为已初始化', () => {
    markInitializationComplete()
    expect(localStorage.setItem).toHaveBeenCalledWith(INIT_STATUS_KEY, 'true')
    expect(isInitialized()).toBe(true)
  })

  it('INIT_STATUS_KEY 是稳定的常量', () => {
    expect(INIT_STATUS_KEY).toBe('system_initialized')
  })
})

describe('map 工具 - 平台标签', () => {
  it('getPlatformTag 返回正确样式', () => {
    expect(getPlatformTag('1')).toBe('success')
    expect(getPlatformTag('4')).toBe('info')
  })

  it('getPlatformTag 未知类型返回 "未知"', () => {
    expect(getPlatformTag('99')).toBe('未知')
  })

  it('getPlatformName 返回正确中文名', () => {
    expect(getPlatformName('1')).toBe('小红书')
    expect(getPlatformName('2')).toBe('视频号')
    expect(getPlatformName('3')).toBe('抖音')
    expect(getPlatformName('4')).toBe('快手')
  })

  it('getPlatformName 未知类型返回 "未知"', () => {
    expect(getPlatformName('99')).toBe('未知')
  })

  it('getPlatformMap 返回 4 个平台', () => {
    const map = getPlatformMap()
    expect(map).toHaveLength(4)
    expect(map[0].value).toBe('1')
    expect(map[3].label).toBe('快手')
  })
})

describe('map 工具 - 状态标签', () => {
  it('getStatusTag 各状态正确', () => {
    expect(getStatusTag('1')).toBe('info')
    expect(getStatusTag('5')).toBe('warning')
    expect(getStatusTag('8')).toBe('success')
    expect(getStatusTag('10')).toBe('danger')
  })

  it('getStatusName 各状态名称正确', () => {
    expect(getStatusName('1')).toBe('待执行')
    expect(getStatusName('5')).toBe('进行中')
    expect(getStatusName('8')).toBe('已失败')
    expect(getStatusName('9')).toBe('已取消')
    expect(getStatusName('10')).toBe('已完成')
  })
})

describe('map 工具 - 线索渠道', () => {
  it('getClueMap 返回 6 个渠道', () => {
    const map = getClueMap()
    expect(map).toHaveLength(6)
  })

  it('getClueName 已知渠道正确', () => {
    expect(getClueName('1')).toBe('QQ')
    expect(getClueName('2')).toBe('微信')
    expect(getClueName('3')).toBe('电话')
    expect(getClueName('4')).toBe('Telegram')
    expect(getClueName('5')).toBe('Whatsapp')
    expect(getClueName('6')).toBe('twitter')
  })

  it('getClueName 未知类型返回 "未知"', () => {
    expect(getClueName('99')).toBe('未知')
  })
})
