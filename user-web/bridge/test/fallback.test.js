// fallback 工具测试
//   - isSelfMessage 自他判定兜底（class、头像位置、data 属性）
//   - deriveAccountId 派生顺序
import { describe, it, expect, vi } from 'vitest';
import { isSelfMessage, deriveAccountId } from '../src/core/fallback.js';

function makeEl({ cls = '', left = 0, width = 100, avatarLeft = 0, avatarWidth = 20, dataSender = '' } = {}) {
  return {
    classList: {
      contains: (c) => cls.split(/\s+/).includes(c),
    },
    dataset: { sender: dataSender, isSelf: '' },
    querySelector: (sel) => {
      if (sel.includes('avatar')) {
        return {
          getBoundingClientRect: () => ({ left: avatarLeft, width: avatarWidth, right: avatarLeft + avatarWidth }),
        };
      }
      return null;
    },
    getBoundingClientRect: () => ({ left, width, right: left + width }),
    matches: (sel) => {
      if (sel === '.right') return cls.split(/\s+/).includes('right');
      return false;
    },
  };
}

describe('fallback / isSelfMessage', () => {
  it('matches 选择器命中时判 self', () => {
    const el = makeEl({ cls: 'right' });
    expect(isSelfMessage(el, '.right')).toBe(true);
  });
  it('querySelector 命中时判 self', () => {
    const el = {
      matches: () => false,
      querySelector: (sel) => (sel === '.right' ? {} : null),
      dataset: {},
      getBoundingClientRect: () => ({ left: 0, width: 0 }),
    };
    expect(isSelfMessage(el, '.right')).toBe(true);
  });
  it('头像在容器中线右侧时判 self（兜底）', () => {
    const el = makeEl({ left: 0, width: 200, avatarLeft: 150, avatarWidth: 20 });
    // 容器中线 = 100；avatar center = 160 > 100 → self
    expect(isSelfMessage(el, '.some-unmatched')).toBe(true);
  });
  it('头像在容器中线左侧时判 other', () => {
    const el = makeEl({ left: 0, width: 200, avatarLeft: 30, avatarWidth: 20 });
    // 容器中线 = 100；avatar center = 40 < 100 → other
    expect(isSelfMessage(el, '.some-unmatched')).toBe(false);
  });
  it('data-sender=self 兜底', () => {
    const el = {
      matches: () => false,
      querySelector: () => null,
      dataset: { sender: 'self' },
      getBoundingClientRect: () => ({ left: 0, width: 0 }),
    };
    expect(isSelfMessage(el, '.none')).toBe(true);
  });
  it('data-is-self=true 兜底', () => {
    const el = {
      matches: () => false,
      querySelector: () => null,
      dataset: { isSelf: 'true' },
      getBoundingClientRect: () => ({ left: 0, width: 0 }),
    };
    expect(isSelfMessage(el, '.none')).toBe(true);
  });
  it('null 元素安全返回 false', () => {
    expect(isSelfMessage(null, '.x')).toBe(false);
  });
});

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
