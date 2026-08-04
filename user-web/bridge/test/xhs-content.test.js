// xhs 消息解析与噪音过滤单测（对齐 douyin 的能力边界）
// 覆盖：
//   1. parseMessageItem 消息类型提取：文本 / 卡片(笔记) / 图片 / 语音 / 撤回 / 聚光系统消息
//   2. 自/他判定：.left(对方) / .right(自己)
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

  it('自己发送（.right）→ sender_type=self', () => {
    const a = setup();
    const parsed = a.parseMessageItem(msgItem('<div class="right"><div class="text-message">收到</div></div>'));
    expect(parsed.sender_type).toBe('self');
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

  it('撤回消息 → msg_type=recall', () => {
    const a = setup();
    const parsed = a.parseMessageItem(msgItem('<div class="left"><div class="text-message">撤回了一条消息</div></div>'));
    expect(parsed.msg_type).toBe('recall');
  });

  it('聚光进线（source-tip）→ msg_type=system', () => {
    const a = setup();
    const parsed = a.parseMessageItem(
      msgItem('<div class="left"><div class="source-tip">来自聚光</div><div class="text-message">你好</div></div>')
    );
    expect(parsed.msg_type).toBe('system');
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