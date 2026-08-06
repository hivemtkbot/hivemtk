// fallback 工具测试
//   - deriveAccountId 派生顺序
import { describe, it, expect, vi } from 'vitest';
import { deriveAccountId } from '../src/core/fallback.js';

describe('fallback / deriveAccountId', () => {
  it('按顺序选第一个非空候选', () => {
    expect(deriveAccountId('douyin_web', ['', null, 'user-1', 'user-2'])).toBe('user-1');
  });
  it('全部空时返回带渠道名的 unknown 字符串', () => {
    expect(deriveAccountId('xhs_web', ['', null, undefined])).toBe('xhs_web-unknown');
  });
  it('trim 前后空白', () => {
    expect(deriveAccountId('douyin_web', ['  user-1  '])).toBe('user-1');
  });
});
