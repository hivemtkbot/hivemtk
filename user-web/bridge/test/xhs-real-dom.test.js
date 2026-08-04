// 回归测试：复现「左侧会话列表已获取，但右侧聊天内容未上报」——用用户实测的真实 .chat-item DOM。
// 真实结构（用户贴的 HTML）：
//   .xhs-im-conv-item.xhs-im-conv-item--active[data-conv-id]  ← 左侧活动会话（已能枚举）
//   .xhs-im-msg-list > .xhs-im-msg-list__time-divider + .chat-item[data-message-id]
//     .chat-item__content.chat-item__content--left            ← 对方消息
//       .chat-item__bubble.chat-item__bubble--text.chat-item__bubble--other
//         .xhs-im-bubble__text  (文本)
//   .xhs-im-input-bar-editor[contenteditable]                  ← 输入框
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { buildXhsAdapter, getConversationId, getConversationList } from '../src/channels/xhs.js';

function makeVisible(el) {
  Object.defineProperty(el, 'offsetParent', { configurable: true, get: () => ({}) });
  return el;
}

function stubLocation(path) {
  vi.stubGlobal('location', {
    href: `https://www.xiaohongshu.com${path}`,
    pathname: path,
    search: '',
    hostname: 'www.xiaohongshu.com',
    host: 'www.xiaohongshu.com',
  });
}

const REAL_DOM = `
  <div class="xhs-im-view">
    <div class="xhs-im-view__sidebar">
      <div class="xhs-im-conv-list">
        <div class="xhs-im-conv-list__scroll">
          <div class="xhs-im-conv-item xhs-im-conv-item--active" data-conv-id="63bd52380000000027029f4d" data-conv-kind="c2c">
            <div class="xhs-im-conv-item__content">
              <div class="xhs-im-conv-item__top"><span class="xhs-im-conv-item__name">雪大王</span></div>
            </div>
          </div>
          <div class="xhs-im-conv-item" data-conv-id="5e4f75e30000000001001cd8" data-conv-kind="c2c">
            <div class="xhs-im-conv-item__content">
              <div class="xhs-im-conv-item__top"><span class="xhs-im-conv-item__name">你黄黄</span></div>
            </div>
          </div>
        </div>
      </div>
    </div>
    <div class="xhs-im-view__main">
      <div class="xhs-im-chat-window">
        <div class="xhs-im-chat-window__header">
          <div class="xhs-im-chat-window__header-text"><span class="xhs-im-chat-window__header-name">雪大王</span></div>
        </div>
        <div class="xhs-im-msg-list-wrap">
          <div class="xhs-im-msg-list">
            <div class="xhs-im-msg-list__time-divider">12:14</div>
            <div class="chat-item" data-store-id="1" data-message-id="63bd52380000000027029f4d.69c730300000000034018cb2.1ea7167300ad22f" data-content-type="1">
              <div class="chat-item__content chat-item__content--left">
                <div class="chat-item__body chat-item__body--left">
                  <div class="chat-item__bubble-row chat-item__bubble-row--left">
                    <div class="chat-item__bubble chat-item__bubble--text chat-item__bubble--other">
                      <div class="xhs-im-bubble__text-wrapper"><p class="xhs-im-bubble__text">95878728570</p></div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div class="xhs-im-input-bar">
        <div class="xhs-im-input-bar-editor" contenteditable="true" data-placeholder="发消息..."></div>
      </div>
    </div>
  </div>`;

describe('小红书真实 .chat-item DOM：右侧消息上报', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    vi.unstubAllGlobals();
  });

  it('左侧会话列表能枚举（data-conv-id）', () => {
    stubLocation('/chat/63bd52380000000027029f4d');
    document.body.innerHTML = REAL_DOM;
    document.querySelectorAll('.xhs-im-conv-item').forEach((el) => makeVisible(el));
    const list = getConversationList();
    expect(list.length).toBe(2);
    expect(list[0].id).toBe('63bd52380000000027029f4d');
    expect(list[0].name).toBe('雪大王');
  });

  it('当前会话 id 来自活动项 data-conv-id', () => {
    stubLocation('/chat/63bd52380000000027029f4d');
    document.body.innerHTML = REAL_DOM;
    document.querySelectorAll('.xhs-im-conv-item').forEach((el) => makeVisible(el));
    expect(getConversationId()).toBe('63bd52380000000027029f4d');
  });

  it('右侧 .chat-item 消息被捕获并上报（会话 id 正确、时间戳被过滤）', () => {
    stubLocation('/chat/63bd52380000000027029f4d');
    document.body.innerHTML = REAL_DOM;
    document.querySelectorAll('.xhs-im-conv-item, .chat-item, .xhs-im-input-bar-editor').forEach((el) => makeVisible(el));

    const adapter = buildXhsAdapter();
    const inbound = [];
    const history = [];
    adapter.start({
      onInbound: (m) => inbound.push(m),
      onHistory: (m) => history.push(m),
    });
    adapter._scanIncremental();

    const captured = inbound.concat(history);
    expect(captured.length).toBeGreaterThan(0);
    const allTexts = captured.flatMap((m) => {
      if (m.history && m.history.length) return m.history.map((h) => h.content);
      return [m.content || ''];
    });
    expect(allTexts).toContain('95878728570');      // 真实消息内容被捕获
    expect(allTexts).not.toContain('12:14');        // 时间分隔被过滤
    // 会话 id 正确（来自活动项 data-conv-id）
    const frame = captured[0];
    const convId = frame.conversation_id;
    expect(convId).toBe('63bd52380000000027029f4d');
  });
});
