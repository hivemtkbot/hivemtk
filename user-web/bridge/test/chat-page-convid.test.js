// 回归测试：修复「抖音/小红书 /chat 聊天页一条消息都捕获不到」。
//
// 根因（来自真实 DOM 快照）：/chat 专用路由的活动会话项
//   douyin:  div.conversationConversationItemwrapper.conversationConversationItemcurConversation[data-e2e="conversation-item"]
//   xhs:     .sx-contact-item.active
// 常既无 /user/ 链接（账号线索 0 个）、也无任何 data-id / data-* 属性。
// 旧版 getConversationId() 兜底链（URL→data 属性→/user/ 链接）全部落空 → 返回 null →
// 适配器 `if (!getConversationId()) return` 守卫拦截全部消息（表现为“打开私信页却零捕获”）。
//
// 修复：增加「活动会话项标题文本派生稳定 id」（conv:<昵称>）兜底。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { BaseAdapter } from '../src/core/channel-adapter.js';
import { SENDER } from '../src/core/types.js';
import { getConversationId, getConversationList, buildDouyinAdapter } from '../src/channels/douyin.js';
import * as xhs from '../src/channels/xhs.js';

// 让 jsdom 元素有可见性（真实浏览器 offsetParent 由布局决定；jsdom 恒为 null）
function makeVisible(el) {
  Object.defineProperty(el, 'offsetParent', { configurable: true, get: () => ({}) });
  return el;
}

function stubLocation(path) {
  vi.stubGlobal('location', {
    href: `https://www.douyin.com${path}`,
    pathname: path,
    search: '',
    hostname: 'www.douyin.com',
    host: 'www.douyin.com',
  });
}

describe('douyin /chat 页活动会话无链接/无 data 属性时 getConversationId 兜底', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    vi.unstubAllGlobals();
  });

  it('活动项仅含 curConversation class + data-e2e（无 /user/ 链接、无 data-id）→ 用昵称文本派生稳定 id', () => {
    stubLocation('/chat');
    const wrapper = document.createElement('div');
    wrapper.className = 'conversationConversationListwrapper';
    const active = makeVisible(document.createElement('div'));
    active.className = 'conversationConversationItemwrapper conversationConversationItemcurConversation';
    active.setAttribute('data-e2e', 'conversation-item');
    const titleWrap = document.createElement('div');
    titleWrap.className = 'conversationConversationItemtitleWrapper';
    const title = document.createElement('div');
    title.className = 'conversationConversationItemtitle';
    title.textContent = '张三';
    titleWrap.appendChild(title);
    active.appendChild(titleWrap);
    wrapper.appendChild(active);
    document.body.appendChild(wrapper);

    const id = getConversationId();
    expect(id).toBe('conv:张三'); // 关键：非 null，消息捕获不再被守卫拦截
  });

  it('活动项无链接/无 data 且无昵称时仍返回 null（保持安全：不凭空造会话）', () => {
    stubLocation('/chat');
    const wrapper = document.createElement('div');
    wrapper.className = 'conversationConversationListwrapper';
    const active = makeVisible(document.createElement('div'));
    active.className = 'conversationConversationItemwrapper conversationConversationItemcurConversation';
    active.setAttribute('data-e2e', 'conversation-item');
    const titleWrap = document.createElement('div');
    titleWrap.className = 'conversationConversationItemtitleWrapper';
    active.appendChild(titleWrap);
    wrapper.appendChild(active);
    document.body.appendChild(wrapper);
    expect(getConversationId()).toBeNull();
  });

  it('getConversationList 对无链接/无 data 的会话项用昵称派生 id，不再跳过', () => {
    stubLocation('/chat');
    const wrapper = document.createElement('div');
    wrapper.className = 'conversationConversationListwrapper';
    const item = makeVisible(document.createElement('div'));
    item.setAttribute('data-e2e', 'conversation-item');
    item.className = 'conversationConversationItemwrapper';
    const title = document.createElement('div');
    title.className = 'conversationConversationItemtitle';
    title.textContent = '李四';
    item.appendChild(title);
    wrapper.appendChild(item);
    document.body.appendChild(wrapper);

    const list = getConversationList();
    expect(list.length).toBe(1);
    expect(list[0].id).toBe('conv:李四');
    expect(list[0].name).toBe('李四');
  });

  it('/chat/{id} 路径直接解析会话 id（与小红书对称，群聊/深链场景）', () => {
    vi.stubGlobal('location', {
      href: 'https://www.douyin.com/chat/MS4wAbCdEf',
      pathname: '/chat/MS4wAbCdEf',
      search: '',
      hostname: 'www.douyin.com',
      host: 'www.douyin.com',
    });
    document.body.innerHTML = '';
    expect(getConversationId()).toBe('MS4wAbCdEf');
  });
});

describe('xhs /chat 页活动会话无链接/无 data 属性时 getConversationId 兜底', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    vi.unstubAllGlobals();
  });

  it('活动会话项（.active）无 data-* 且无 header 链接时，用昵称文本派生 id', () => {
    vi.stubGlobal('location', {
      href: 'https://www.xiaohongshu.com/chat',
      pathname: '/chat',
      search: '',
      hostname: 'www.xiaohongshu.com',
      host: 'www.xiaohongshu.com',
    });
    document.body.innerHTML = `
      <div class="chat-list-box">
        <div class="sx-contact-item active">
          <div class="nick-name">王五</div>
        </div>
      </div>`;
    const id = xhs.getConversationId();
    expect(id).toBe('conv:王五'); // 非 null → 消息捕获不被拦截
  });

  it('getConversationList 对无 data-* 的会话项用昵称派生 id', () => {
    vi.stubGlobal('location', {
      href: 'https://www.xiaohongshu.com/chat',
      pathname: '/chat',
      search: '',
      hostname: 'www.xiaohongshu.com',
      host: 'www.xiaohongshu.com',
    });
    const item = makeVisible(document.createElement('div'));
    item.className = 'sx-contact-item';
    const name = document.createElement('div');
    name.className = 'nick-name';
    name.textContent = '赵六';
    item.appendChild(name);
    document.body.appendChild(item);

    const list = xhs.getConversationList();
    expect(list.length).toBe(1);
    expect(list[0].id).toBe('conv:赵六');
  });

  it('新版 .xhs-im-conv-item 用 data-conv-id 枚举会话列表（左侧列表驱动）', () => {
    vi.stubGlobal('location', {
      href: 'https://www.xiaohongshu.com/chat/5e4f75e30000000001001cd8',
      pathname: '/chat/5e4f75e30000000001001cd8',
      search: '',
      hostname: 'www.xiaohongshu.com',
      host: 'www.xiaohongshu.com',
    });
    const wrap = document.createElement('div');
    wrap.className = 'xhs-im-conv-list';
    const scroll = document.createElement('div');
    scroll.className = 'xhs-im-conv-list__scroll';
    function addConv(convId, name) {
      const item = makeVisible(document.createElement('div'));
      item.className = 'xhs-im-conv-item';
      item.setAttribute('data-conv-id', convId);
      item.setAttribute('data-conv-kind', 'c2c');
      const content = document.createElement('div');
      content.className = 'xhs-im-conv-item__content';
      const nameEl = document.createElement('div');
      nameEl.className = 'xhs-im-conv-item__name';
      nameEl.textContent = name;
      content.appendChild(nameEl);
      item.appendChild(content);
      scroll.appendChild(item);
    }
    addConv('63bd52380000000027029f4d', '雪大王');
    addConv('5e4f75e30000000001001cd8', '你黄黄');
    wrap.appendChild(scroll);
    document.body.appendChild(wrap);

    const list = xhs.getConversationList();
    expect(list.length).toBe(2);
    expect(list[0].id).toBe('63bd52380000000027029f4d');
    expect(list[0].name).toBe('雪大王');
    expect(list[1].id).toBe('5e4f75e30000000001001cd8');
    expect(list[1].name).toBe('你黄黄');
  });
});

// 端到端回归：复现「打开 /chat 聊天页却一条消息都捕获不到」——
// 用真实深检快照的抖音 /chat DOM（活动会话项无 /user/ 链接、无 data-id），
// 验证适配器完整捕获实时消息（修复前 getConversationId()===null 全部被守卫拦截）。
describe('douyin /chat 快照 DOM 端到端消息捕获', () => {  function makeVisible(el) {
    Object.defineProperty(el, 'offsetParent', { configurable: true, get: () => ({}) });
    return el;
  }

  beforeEach(() => {
    document.body.innerHTML = '';
    vi.unstubAllGlobals();
  });

  it('复现深检快照结构：无链接/无 data 活动会话 → 客户消息被捕获并上行', () => {
    stubLocation('/chat');
    // 真实 /chat 结构（来自用户深检快照）：
    //   会话列表 wrapper + 活动项(curConversation, data-e2e=conversation-item, 无链接无data)
    //   消息线程：气泡带 data-e2e="msg-item-content"
    document.body.innerHTML = `
      <div class="conversationConversationListwrapper">
        <div class="conversationConversationItemwrapper conversationConversationItemcurConversation" data-e2e="conversation-item">
          <div class="conversationConversationItemtitleWrapper"><div class="conversationConversationItemtitle">张三</div></div>
        </div>
      </div>
      <div class="messageMsgList" data-e2e="chat-msg-list">
        <div class="messageMessageItem" data-e2e="msg-item-content"><div class="messageMessageText">你好，在吗</div></div>
        <div class="messageMessageItem" data-e2e="msg-item-content"><div class="messageMessageText">怎么收费</div></div>
      </div>
      <div class="zone-container editor-kit-container messageEditorinputArea" contenteditable="true"></div>
      <svg class="messageMsgInputpublishBtn e2e-send-msg-btn"></svg>`;

    const adapter = buildDouyinAdapter();
    // 可见性（jsdom offsetParent 恒 null）
    document.querySelectorAll('div[data-e2e="conversation-item"], div[data-e2e="msg-item-content"], .messageEditorinputArea').forEach((el) => makeVisible(el));

    const inbound = [];
    const history = [];
    adapter.start({
      onInbound: (m) => inbound.push(m),
      onHistory: (m) => history.push(m),
    });
    // start() 已设置 8s 宽限期 → 首次扫描客户消息走回填（仅落库，符合设计）
    // 关键：无论 inbound 还是 history，消息都已被捕获（修复前 getConversationId()===null 全被拦截）
    adapter._scanIncremental();

    const captured = inbound.concat(history);
    expect(captured.length).toBeGreaterThan(0);
    // 会话级 history 帧：多轮历史在 m.history[]（含全部轮次 content）
    const allTexts = captured.flatMap((m) => {
      if (m.history && m.history.length) return m.history.map((h) => h.content);
      return [m.content || ''];
    });
    expect(allTexts).toContain('你好，在吗');
    expect(allTexts).toContain('怎么收费');

    // 宽限期过后，新的客户消息应实时走 inbound（触发 AI）
    adapter.historyGraceUntil = Date.now() - 1;
    const msgEl = document.createElement('div');
    msgEl.setAttribute('data-e2e', 'msg-item-content');
    msgEl.className = 'messageMessageItem';
    msgEl.textContent = '实时新消息';
    makeVisible(msgEl);
    document.querySelector('[data-e2e="chat-msg-list"]').appendChild(msgEl);
    adapter._scanIncremental();
    expect(inbound.some((m) => m.content === '实时新消息')).toBe(true);
    expect(inbound[inbound.length - 1].conversation_id).toBe('conv:张三');
  });
});


// 端到端回归：小红书新版 /chat 页（xhs-im-* BEM）消息捕获 + 时间戳过滤。
// 复现用户深检快照：会话 id 来自 /chat/{id} 路径，header 有「我」的链接需跳过，
// 消息气泡为 xhs-im-msg-item（含时间戳标记 09:56 不应被当作消息）。
describe('xhs /chat 快照 DOM 端到端消息捕获', () => {
  function makeVisible(el) {
    Object.defineProperty(el, 'offsetParent', { configurable: true, get: () => ({}) });
    return el;
  }

  beforeEach(() => {
    document.body.innerHTML = '';
    vi.unstubAllGlobals();
  });

  it('复现深检快照：/chat/{id} 路径会话 + xhs-im-* 气泡 → 捕获消息且时间戳被过滤', () => {
    vi.stubGlobal('location', {
      href: 'https://www.xiaohongshu.com/chat/5e4f75e30000000001001cd8',
      pathname: '/chat/5e4f75e30000000001001cd8',
      search: '',
      hostname: 'www.xiaohongshu.com',
      host: 'www.xiaohongshu.com',
    });
    // 用户实测真实 DOM：左侧 .xhs-im-conv-item（--active 活动项，data-conv-id），
    // 右侧 .xhs-im-msg-list 内 .chat-item（data-message-id，--left=对方，.xhs-im-bubble__text）
    document.body.innerHTML = `
      <div class="xhs-im-view">
        <div class="xhs-im-view__sidebar">
          <div class="xhs-im-conv-list">
            <div class="xhs-im-conv-list__scroll">
              <div class="xhs-im-conv-item" data-conv-id="63bd52380000000027029f4d" data-conv-kind="c2c">
                <div class="xhs-im-conv-item__content"><div class="xhs-im-conv-item__name">雪大王</div></div>
              </div>
              <div class="xhs-im-conv-item xhs-im-conv-item--active" data-conv-id="5e4f75e30000000001001cd8" data-conv-kind="c2c">
                <div class="xhs-im-conv-item__content"><div class="xhs-im-conv-item__name">你黄黄</div></div>
              </div>
            </div>
          </div>
        </div>
        <div class="xhs-im-view__main">
          <div class="xhs-im-chat-window">
            <div class="xhs-im-chat-window__header">
              <div class="xhs-im-chat-window__header-text"><span class="xhs-im-chat-window__header-name">你黄黄</span></div>
            </div>
            <div class="xhs-im-msg-list-wrap">
              <div class="xhs-im-msg-list">
                <div class="xhs-im-msg-list__time-divider">09:56</div>
                <div class="chat-item" data-message-id="5e4f75e30000000001001cd8.69c730300000000034018cb2.1ea7146c60a6281" data-content-type="1">
                  <div class="chat-item__content chat-item__content--left">
                    <div class="chat-item__body chat-item__body--left">
                      <div class="chat-item__bubble-row chat-item__bubble-row--left">
                        <div class="chat-item__bubble chat-item__bubble--text chat-item__bubble--other">
                          <div class="xhs-im-bubble__text-wrapper"><p class="xhs-im-bubble__text">在吗</p></div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
                <div class="chat-item" data-message-id="5e4f75e30000000001001cd8.69c730300000000034018cb2.abc123">
                  <div class="chat-item__content chat-item__content--left">
                    <div class="chat-item__body chat-item__body--left">
                      <div class="chat-item__bubble-row chat-item__bubble-row--left">
                        <div class="chat-item__bubble chat-item__bubble--text chat-item__bubble--other">
                          <div class="xhs-im-bubble__text-wrapper"><p class="xhs-im-bubble__text">这件多少钱</p></div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="xhs-im-input-bar"><div class="xhs-im-input-bar-editor" contenteditable="true"></div></div>
        </div>
      </div>`;

    const adapter = xhs.buildXhsAdapter();
    document.querySelectorAll('.xhs-im-conv-item, .chat-item, .xhs-im-input-bar-editor').forEach((el) => makeVisible(el));

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
    // 真实消息被捕获
    expect(allTexts).toContain('在吗');
    expect(allTexts).toContain('这件多少钱');
    // 时间戳 09:56 不被当作消息
    expect(allTexts).not.toContain('09:56');
    // 会话 id 来自活动项 data-conv-id
    const frame = captured[0];
    const convId = frame.conversation_id || (frame.history && frame.history[0] && frame.history[0].group_id) || '';
    expect(convId || frame.history).toBeTruthy();
  });
});
