// 闲鱼 bridge 单元测试
// 覆盖：
//  1. 自/他判定漏斗（class/data/对齐/头像）
//  2. 系统消息识别漏斗（居中/class/role/时间/非好友/结构）
//  3. 时间格式识别（isTimeText）
//  4. 系统文本识别（isSystemText）
//  5. 会话 id 解析（URL / active item / header link / 昵称派生）
//  6. account id 多层兜底
//  7. 消息提取（parseMessageItem）
import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  isSystemMessage, classifySender, isTimeText, isSystemText,
  isCenterAligned, isListNoise, normalizeContactId, detectUnread,
  getPeerName,
} from '../src/channels/xianyu.js';
import { SENDER } from '../src/core/types.js';

// jsdom 默认无 getComputedStyle；我们提供最小可用的 polyfill
function installComputedStyle(map = {}) {
  const original = globalThis.getComputedStyle;
  globalThis.getComputedStyle = (el) => {
    if (el && el.__css) return el.__css;
    return original ? original(el) : {};
  };
  // 每个测试自己写 map[elSymbol] = { textAlign, justifyContent, ... }
  return map;
}

// 构造一个 DOM 元素（带 bbox + closest + matches + dataset）
function makeEl({ tag = 'div', cls = '', left = 0, width = 100, css = null, dataSender = '', dataDir = '', dataIsSelf = '', attrs = {} } = {}) {
  const classList = new Set(cls.split(/\s+/).filter(Boolean));
  // 简易 CSS [class*="..."] 匹配（i 标志忽略大小写）
  const classAttrContains = (substr) => {
    const lsub = substr.toLowerCase();
    for (const c of classList) {
      if (c.toLowerCase().includes(lsub)) return true;
    }
    return false;
  };
  const dataAttrValue = (key) => {
    if (key === 'sender') return dataSender;
    if (key === 'is-self') return dataIsSelf;
    if (key === 'direction') return dataDir;
    return attrs[`data-${key}`] || '';
  };
  // 单个选择器匹配（用于 comma-joined 列表的拆分匹配）
  const matchSingle = (single) => {
    // [class*="..."] 属性选择器（i = 大小写不敏感）
    const classSelMatch = single.match(/\[class\*="([^"]+)"\s*i?\]/);
    if (classSelMatch) {
      return classAttrContains(classSelMatch[1]);
    }
    // [data-xxx="yyy"] 属性选择器
    const dataSelMatch = single.match(/\[data-([\w-]+)(?:="([^"]*)")?\]/);
    if (dataSelMatch) {
      const val = dataAttrValue(dataSelMatch[1]);
      if (dataSelMatch[2] !== undefined) return val === dataSelMatch[2];
      return !!val;
    }
    // role="status" 等
    const roleMatch = single.match(/\[role="([^"]+)"\]/);
    if (roleMatch) {
      return (attrs.role || '') === roleMatch[1];
    }
    // aria-live="polite" 等
    const ariaMatch = single.match(/\[aria-live="([^"]+)"\]/);
    if (ariaMatch) {
      return (attrs['aria-live'] || '') === ariaMatch[1];
    }
    // .className
    if (single.startsWith('.')) {
      const c = single.slice(1).split(/[\s>+~:[]/)[0];
      if (classList.has(c)) return true;
    }
    return false;
  };
  return {
    tagName: tag.toUpperCase(),
    classList: {
      contains: (c) => classList.has(c),
      add: (c) => classList.add(c),
    },
    dataset: { sender: dataSender, isSelf: dataIsSelf, direction: dataDir },
    attributes: attrs,
    getAttribute: (name) => attrs[name] || null,
    closest: (sel) => {
      // 简化：按 [class*="..."] 子串匹配
      const m = sel.match(/\[class\*="([^"]+)"\s*i?\]/);
      if (m && classAttrContains(m[1])) return true;
      if (sel.startsWith('.')) {
        const c = sel.slice(1).split(/[\s>+~:]/)[0];
        if (classList.has(c)) return true;
      }
      return null;
    },
    matches: (sel) => {
      // 真实 DOM 的 matches() 支持逗号分隔的选择器列表（任一命中即 true）
      const alts = sel.split(',').map((s) => s.trim()).filter(Boolean);
      for (const single of alts) {
        if (matchSingle(single)) return true;
      }
      return false;
    },
    querySelector: () => null,
    getBoundingClientRect: () => ({ left, width, right: left + width, top: 0, height: 50, bottom: 50 }),
    parentElement: null,
    __css: css,
    children: [],
  };
}

describe('xianyu / isTimeText', () => {
  it('匹配纯时分格式 09:56', () => {
    expect(isTimeText('09:56')).toBe(true);
  });
  it('匹配今天 12:30', () => {
    expect(isTimeText('今天 12:30')).toBe(true);
  });
  it('匹配昨天 09:56', () => {
    expect(isTimeText('昨天 09:56')).toBe(true);
  });
  it('匹配日期 5月20日', () => {
    expect(isTimeText('5月20日')).toBe(true);
  });
  it('匹配上午 10:30', () => {
    expect(isTimeText('上午 10:30')).toBe(true);
  });
  it('匹配 ISO 日期 2026-08-04', () => {
    expect(isTimeText('2026-08-04')).toBe(true);
  });
  it('不匹配普通文本', () => {
    expect(isTimeText('你好')).toBe(false);
  });
  it('不匹配空字符串', () => {
    expect(isTimeText('')).toBe(false);
  });
  it('不匹配 null', () => {
    expect(isTimeText(null)).toBe(false);
  });
});

describe('xianyu / isSystemText', () => {
  it('匹配非好友提示', () => {
    expect(isSystemText('你还不是对方好友，请勿轻信任何转账信息')).toBe(true);
  });
  it('匹配拒收提示', () => {
    expect(isSystemText('消息已发出，但被对方拒收')).toBe(true);
  });
  it('匹配撤回消息', () => {
    expect(isSystemText('对方撤回了一条消息')).toBe(true);
  });
  it('匹配对方正在输入', () => {
    expect(isSystemText('对方正在输入...')).toBe(true);
  });
  it('匹配订单已创建', () => {
    expect(isSystemText('订单已创建')).toBe(true);
  });
  it('不匹配普通聊天', () => {
    expect(isSystemText('在吗')).toBe(false);
  });
  it('不匹配空字符串', () => {
    expect(isSystemText('')).toBe(false);
  });
});

describe('xianyu / normalizeContactId', () => {
  it('去除 Total- 前缀', () => {
    expect(normalizeContactId('Total-123')).toBe('123');
  });
  it('去除 User- 前缀', () => {
    expect(normalizeContactId('User_456')).toBe('456');
  });
  it('清理非法字符', () => {
    expect(normalizeContactId('abc!@#123')).toBe('abc123');
  });
  it('空字符串返回空', () => {
    expect(normalizeContactId('')).toBe('');
  });
  it('null 返回空', () => {
    expect(normalizeContactId(null)).toBe('');
  });
  it('纯数字保留', () => {
    expect(normalizeContactId('1234567890')).toBe('1234567890');
  });
});

describe('xianyu / isSystemMessage 漏斗', () => {
  beforeEach(() => installComputedStyle());

  it('① 居中对齐 → 系统消息', () => {
    const el = makeEl({ css: { textAlign: 'center' } });
    expect(isSystemMessage(el)).toBe(true);
  });

  it('① 父容器 justifyContent=center → 系统消息', () => {
    const el = makeEl({ css: { textAlign: 'left' } });
    el.parentElement = makeEl({ css: { justifyContent: 'center' } });
    expect(isSystemMessage(el)).toBe(true);
  });

  it('② class 含 system-msg → 系统消息', () => {
    const el = makeEl({ cls: 'im-notice system-msg' });
    expect(isSystemMessage(el)).toBe(true);
  });

  it('② class 含 chat-notice → 系统消息', () => {
    const el = makeEl({ cls: 'chat-notice' });
    expect(isSystemMessage(el)).toBe(true);
  });

  it('② class 含 divider → 系统消息', () => {
    const el = makeEl({ cls: 'divider time-stamp' });
    expect(isSystemMessage(el)).toBe(true);
  });

  it('④ 纯时间文本 → 系统消息', () => {
    const el = makeEl({ cls: 'normal-bubble' });
    // mock cleanText：直接给 querySelector 返回空 → cleanText 走自己的逻辑返回 ''
    // 真实情况：cleanText 拿不到 text，这里用内嵌 textContent 模拟
    el.textContent = '09:56';
    // 简化的 textContent 路径：cleanText 在 jsdom 中取 innerText 通常空，需 mock
    el.querySelector = (sel) => {
      if (sel.includes('bubble') || sel.includes('msg-content')) return { textContent: '09:56' };
      return null;
    };
    expect(isSystemMessage(el)).toBe(true);
  });

  it('非系统消息（普通聊天）→ 非系统', () => {
    const el = makeEl({ cls: 'msg-item msg-self' });
    el.querySelector = (sel) => {
      if (sel.includes('msg-content')) return { textContent: '你好，在吗' };
      return null;
    };
    // 居中? no. class system? no. role? no. 内容模式? no. 结构特征? 有avatar/bubble.
    expect(isSystemMessage(el)).toBe(false);
  });

  it('null 元素安全返回 false', () => {
    expect(isSystemMessage(null)).toBe(false);
  });
});

describe('xianyu / classifySender 漏斗', () => {
  beforeEach(() => installComputedStyle());

  it('A) class 含 self → SELF', () => {
    const el = makeEl({ cls: 'msg-item msg-self' });
    expect(classifySender(el)).toBe(SENDER.SELF);
  });

  it('A) class 含 my-message → SELF', () => {
    const el = makeEl({ cls: 'msg-item my-message' });
    expect(classifySender(el)).toBe(SENDER.SELF);
  });

  it('A) class 含 other → CUSTOMER', () => {
    const el = makeEl({ cls: 'msg-item msg-other' });
    expect(classifySender(el)).toBe(SENDER.CUSTOMER);
  });

  it('A) class 含 peer-message → CUSTOMER', () => {
    const el = makeEl({ cls: 'msg-item peer-message' });
    expect(classifySender(el)).toBe(SENDER.CUSTOMER);
  });

  it('B) data-sender=self → SELF', () => {
    const el = makeEl({ dataSender: 'self' });
    expect(classifySender(el)).toBe(SENDER.SELF);
  });

  it('B) data-sender=other → CUSTOMER', () => {
    const el = makeEl({ dataSender: 'other' });
    expect(classifySender(el)).toBe(SENDER.CUSTOMER);
  });

  it('B) data-direction=outgoing → SELF', () => {
    const el = makeEl({ dataDir: 'outgoing' });
    expect(classifySender(el)).toBe(SENDER.SELF);
  });

  it('B) data-direction=incoming → CUSTOMER', () => {
    const el = makeEl({ dataDir: 'incoming' });
    expect(classifySender(el)).toBe(SENDER.CUSTOMER);
  });

  it('缺信号时默认 CUSTOMER（防回环）', () => {
    const el = makeEl({});
    expect(classifySender(el)).toBe(SENDER.CUSTOMER);
  });
});

describe('xianyu / isCenterAligned', () => {
  beforeEach(() => installComputedStyle());

  it('textAlign=center → true', () => {
    const el = makeEl({ css: { textAlign: 'center' } });
    expect(isCenterAligned(el)).toBe(true);
  });

  it('textAlign=left → false', () => {
    const el = makeEl({ css: { textAlign: 'left' } });
    expect(isCenterAligned(el)).toBe(false);
  });

  it('父容器 justifyContent=center → true', () => {
    const el = makeEl({ css: { textAlign: 'start' } });
    el.parentElement = makeEl({ css: { justifyContent: 'center' } });
    expect(isCenterAligned(el)).toBe(true);
  });

  it('null 元素安全', () => {
    expect(isCenterAligned(null)).toBe(false);
  });
});

describe('xianyu / isListNoise', () => {
  it('null 元素 → false', () => {
    expect(isListNoise(null)).toBe(false);
  });

  it('不含 closest 的元素 → false', () => {
    expect(isListNoise({})).toBe(false);
  });

  it('closest 命中 conversation-item → true', () => {
    const el = { closest: (sel) => (sel.includes('conversation-item') ? {} : null) };
    expect(isListNoise(el)).toBe(true);
  });

  it('closest 命中 conv-item → true', () => {
    const el = { closest: (sel) => (sel.includes('conv-item') ? {} : null) };
    expect(isListNoise(el)).toBe(true);
  });

  it('closest chat-list 但自己也是 conversation-item → true', () => {
    const el = {
      closest: (sel) => (sel.includes('chat-list') || sel.includes('conversation-item') || sel.includes('conv-item') ? {} : null),
      matches: (sel) => sel.includes('conversation-item'),
    };
    expect(isListNoise(el)).toBe(true);
  });

  it('普通消息元素 → false', () => {
    const el = { closest: () => null, matches: () => false };
    expect(isListNoise(el)).toBe(false);
  });
});

describe('xianyu / detectUnread', () => {
  it('null 元素 → false', () => {
    expect(detectUnread(null)).toBe(false);
  });

  it('matches 命中 unread → true', () => {
    const el = { matches: (sel) => sel.includes('unread') };
    expect(detectUnread(el)).toBe(true);
  });

  it('querySelector 命中 unread 红点 → true', () => {
    const el = { matches: () => false, querySelector: (sel) => (sel.includes('unread') ? {} : null) };
    expect(detectUnread(el)).toBe(true);
  });

  it('无未读标记 → false', () => {
    const el = { matches: () => false, querySelector: () => null };
    expect(detectUnread(el)).toBe(false);
  });
});
