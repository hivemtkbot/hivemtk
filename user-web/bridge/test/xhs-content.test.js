// xhs 消息解析与噪音过滤单测（对齐 douyin 的能力边界）
// 覆盖：
//   1. parseMessageItem 消息类型提取：文本 / 卡片(笔记) / 图片 / 语音 / 撤回 / 聚光系统消息
//   2. 自/他判定（2026-08-06 已废弃：前端不再计算 self/other，统一 customer；后端权威重判）
//   3. 噪音过滤：会话列表项 .sx-contact-item 绝不当聊天内容解析
//   4. 头像图不误判为消息图片（每条私信都带发送者头像）
import { describe, it, expect, beforeEach } from 'vitest';
import { buildXhsAdapter } from '../src/channels/xhs.js';

function setup(activeContact = true) {
  const adapter = buildXhsAdapter();
  // 固定活动会话：让 parseMessageItem 的 conversation_id 兜底稳定
  if (activeContact) {
    const c = document.createElement('div');
    c.className = 'sx-contact-item active';
    c.setAttribute('data-id', 'conv1');
    document.body.appendChild(c);
  }
  return adapter;
}

function msgItem(innerHtml) {
  const el = document.createElement('div');
  el.className = 'im-msg-item';
  el.innerHTML = innerHtml;
  return el;
}

beforeEach(() => {
  document.body.innerHTML = '';
});

describe('xhs parseMessageItem 消息类型提取', () => {
  it('普通文本消息 → msg_type=text，sender_type=customer（.left 对方）', () => {
    const a = setup();
    const parsed = a.parseMessageItem(msgItem('<div class="left"><div class="text-message">你好，在吗</div></div>'));
    expect(parsed).not.toBeNull();
    expect(parsed.text).toBe('你好，在吗');
    expect(parsed.msg_type).toBe('text');
    expect(parsed.sender_type).toBe('customer');
    expect(parsed.media_url).toBe('');
  });

  it('自己发送（.right）→ 前端不再判定 self/other，统一 sender_type=customer（后端权威重判）', () => {
    const a = setup();
    const parsed = a.parseMessageItem(msgItem('<div class="right"><div class="text-message">收到</div></div>'));
    expect(parsed.sender_type).toBe('customer');
  });

  it('图片消息（含非头像 img，无文本）→ msg_type=image + media_url', () => {
    const a = setup();
    const parsed = a.parseMessageItem(
      msgItem('<div class="left"><img class="image-item" src="https://img.xhs.com/a.jpg"></div>')
    );
    expect(parsed.msg_type).toBe('image');
    expect(parsed.media_url).toBe('https://img.xhs.com/a.jpg');
  });

  it('文本消息带发送者头像时，头像图不误判为图片消息', () => {
    const a = setup();
    const parsed = a.parseMessageItem(
      msgItem('<div class="left"><img class="avatar" src="https://img.xhs.com/av.jpg"><div class="text-message">仅文本</div></div>')
    );
    expect(parsed.msg_type).toBe('text');
    expect(parsed.media_url).toBe('');
  });

  it('笔记卡片消息 → msg_type=card，文本取卡片 info/title', () => {
    const a = setup();
    const parsed = a.parseMessageItem(
      msgItem('<div class="left"><div class="card_container"><div class="card_bottom_title">XX 笔记</div><div class="card_bottom_info">详情见笔记</div></div></div>')
    );
    expect(parsed.msg_type).toBe('card');
    expect(parsed.text).toContain('详情见笔记');
  });

  it('撤回消息 → msg_type=system, sender_type=system（不再触发 AI）', () => {
    const a = setup();
    const parsed = a.parseMessageItem(msgItem('<div class="left"><div class="text-message">撤回了一条消息</div></div>'));
    // 2026-08-05 修复：撤回消息统一标记为 system 类型，sender_type=system
    expect(parsed.msg_type).toBe('system');
    expect(parsed.sender_type).toBe('system');
    // msg_id 兜底应为 sys:${text}，不依赖 textContent.length / Date.now()
    expect(parsed.message_id).toBe('sys:撤回了一条消息');
    expect(parsed.media_url).toBe('');
  });

  it('聚光进线（source-tip）→ msg_type=system, sender_type=system', () => {
    const a = setup();
    const parsed = a.parseMessageItem(
      msgItem('<div class="left"><div class="source-tip">来自聚光</div><div class="text-message">你好</div></div>')
    );
    expect(parsed.msg_type).toBe('system');
    expect(parsed.sender_type).toBe('system');
    // extractMessageContent 的 text 取自 .text-message 元素 = "你好"
    expect(parsed.text).toBe('你好');
    expect(parsed.message_id).toBe(`sys:${parsed.text}`);
  });

  it('系统消息优先使用 data-message-id（兜底才走 sys:${text}）', () => {
    const a = setup();
    const el = document.createElement('div');
    el.className = 'im-msg-item';
    el.setAttribute('data-message-id', 'sys-001');
    el.innerHTML = '<div class="source-tip">来自聚光</div><div class="text-message">你好</div>';
    const parsed = a.parseMessageItem(el);
    expect(parsed.msg_type).toBe('system');
    expect(parsed.sender_type).toBe('system');
    expect(parsed.message_id).toBe('sys-001');
  });

  it('系统消息即使 .left 对方标记存在，sender_type 仍为 system（不被 isSelfMessage 覆盖）', () => {
    const a = setup();
    // 即便 DOM 里有 .left（对方）标记，系统消息分支优先，sender_type 必须为 system
    const parsed = a.parseMessageItem(
      msgItem('<div class="left"><div class="source-tip">系统通知</div></div>')
    );
    expect(parsed.msg_type).toBe('system');
    expect(parsed.sender_type).toBe('system');
  });
});

describe('xhs 系统消息 msg_id 跨轮询稳定性', () => {
  // 2026-08-05 修复回归测试：同一 DOM 系统消息节点每轮扫描应生成相同 message_id，
  // 后端 GetByMsgID 才能正确幂等去重，避免「无新消息仍不断回复 AI」的根因复发。
  it('同一系统消息 DOM 节点两次解析 → message_id 完全相同', () => {
    const a = setup();
    const el = msgItem('<div class="left"><div class="source-tip">来自聚光</div><div class="text-message">你好</div></div>');
    const parsed1 = a.parseMessageItem(el);
    const parsed2 = a.parseMessageItem(el);
    expect(parsed1).not.toBeNull();
    expect(parsed2).not.toBeNull();
    expect(parsed1.message_id).toBe(parsed2.message_id);
    expect(parsed1.message_id).toBe(`sys:${parsed1.text}`);
  });

  it('系统消息 message_id 不含 Date.now() / textContent.length（稳定性核心）', () => {
    const a = setup();
    const el = msgItem('<div class="left"><div class="source-tip">来自聚光</div><div class="text-message">你好</div></div>');
    const parsed = a.parseMessageItem(el);
    // message_id 应为 sys:${text}，不应包含纯数字串（Date.now）或数字后缀
    expect(parsed.message_id).toMatch(/^sys:/);
    expect(parsed.message_id).not.toMatch(/\d{10,}/); // 不含 unix timestamp
  });

  it('不同文本的系统消息 → message_id 不同（避免误合并）', () => {
    const a = setup();
    const p1 = a.parseMessageItem(msgItem('<div class="left"><div class="source-tip">来自聚光A</div></div>'));
    const p2 = a.parseMessageItem(msgItem('<div class="left"><div class="source-tip">来自聚光B</div></div>'));
    expect(p1.message_id).not.toBe(p2.message_id);
  });
});

describe('xhs 噪音过滤', () => {
  it('会话列表项（.sx-contact-item）被解析为 null，绝不当聊天内容上行', () => {
    const a = setup(false);
    const contact = document.createElement('div');
    contact.className = 'sx-contact-item';
    contact.innerHTML = '<div class="nick-name">张三</div><div class="content">最近消息</div>';
    document.body.appendChild(contact);
    expect(a.parseMessageItem(contact)).toBeNull();
  });

  it('无内容且无媒体的纯空节点 → null', () => {
    const a = setup();
    const el = document.createElement('div');
    el.className = 'im-msg-item';
    expect(a.parseMessageItem(el)).toBeNull();
  });
});

describe('xhs getMessageItems 只返回消息项（不混入会话列表）', () => {
  it('会话列表 + 消息混合 DOM 下，仅返回 .im-msg-item', () => {
    const a = setup();
    document.body.innerHTML = `
      <div class="chat-list-box">
        <div class="sx-contact-item" data-id="a"><div class="nick-name">张三</div></div>
        <div class="sx-contact-item" data-id="b"><div class="nick-name">李四</div></div>
      </div>
      <div class="im-chat-window">
        <div class="im-msg-item"><div class="left"><div class="text-message">你好</div></div></div>
        <div class="im-msg-item"><div class="left"><div class="text-message">在吗</div></div></div>
      </div>
      <textarea id="jarvis-reply-textarea"></textarea>
    `;
    const items = a.getMessageItems();
    expect(items.length).toBeGreaterThanOrEqual(2);
    expect(items.every((el) => !el.closest('.sx-contact-item'))).toBe(true);
  });
});