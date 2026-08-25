/**
 * 列表工具单元测试
 * 覆盖 list.js 中的 toList 函数
 */
import { describe, it, expect } from 'vitest'
import { toList } from '@/utils/list.js'

describe('toList', () => {
  it('数组输入直接返回', () => {
    const input = [1, 2, 3]
    expect(toList(input)).toEqual([1, 2, 3])
  })

  it('包含 list 字段的对象', () => {
    const input = { list: ['a', 'b'], total: 10 }
    expect(toList(input)).toEqual(['a', 'b'])
  })

  it('包含 items 字段的对象', () => {
    const input = { items: ['x', 'y', 'z'] }
    expect(toList(input)).toEqual(['x', 'y', 'z'])
  })

  it('包含 data 字段的对象', () => {
    const input = { data: [1, 2] }
    expect(toList(input)).toEqual([1, 2])
  })

  it('包含 stages 字段的对象', () => {
    const input = { stages: ['stage1', 'stage2'] }
    expect(toList(input)).toEqual(['stage1', 'stage2'])
  })

  it('空数组返回空数组', () => {
    expect(toList([])).toEqual([])
  })

  it('null 返回空数组', () => {
    expect(toList(null)).toEqual([])
  })

  it('undefined 返回空数组', () => {
    expect(toList(undefined)).toEqual([])
  })

  it('空对象返回空数组', () => {
    expect(toList({})).toEqual([])
  })

  it('字符串返回空数组', () => {
    expect(toList('string')).toEqual([])
  })

  it('数字返回空数组', () => {
    expect(toList(123)).toEqual([])
  })

  it('布尔值返回空数组', () => {
    expect(toList(true)).toEqual([])
  })

  it('单个对象（无数组字段）返回空数组', () => {
    const input = { id: 1, name: 'test' }
    expect(toList(input)).toEqual([])
  })
})
