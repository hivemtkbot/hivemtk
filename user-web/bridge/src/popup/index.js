// popup 逻辑：配置后端地址/鉴权、测试连接、查看连接状态、自检私信页。
//
// 设计要点：
//   - 状态用 banner（持久可见）而非按钮文字闪烁；不再让用户看不到反馈
//   - 所有 chrome.runtime.sendMessage / tabs.sendMessage 都有 lastError 兜底
//   - "测试连接"：fetch /api/health 验证 URL 真实可达
//   - "打开私信页"：直接开新标签，免去用户手动导航
//   - 输入框支持回车提交
//   - 所有默认值/端口/URL 统一从 ../core/constants.js 单源导入（DEVELOPMENT.md 端口对照表）
import {
  DEFAULT_USER_SERVER,
  PLATFORM_ENTRY_URLS,
  UI_DEFAULTS,
} from '../core/constants.js';

const $ = (id) => document.getElementById(id);

// ---- 默认值（全部来自 constants.js，禁止就地写死） ----
// 文档源：user-server/docs/dev/DEVELOPMENT.md 端口对照表 + user-server/Dockerfile ENV SERVER_PORT=8204
//         + user-server/cmd/api/main.go listenAddr 兜底 :8204
// 调整流程：见 docs/bridge/DEFAULTS.md §3
const DEFAULT_PORT_HINT = DEFAULT_USER_SERVER.port;
const DEFAULT_PLACEHOLDER = DEFAULT_USER_SERVER.baseUrl;
const HEALTH_PATHS = DEFAULT_USER_SERVER.healthPaths;
// popup「测试连接」单次 fetch 超时（与 constants.UI_DEFAULTS.healthCheckTimeoutMs 同源）
const POPUP_HEALTH_CHECK_TIMEOUT_MS = UI_DEFAULTS.healthCheckTimeoutMs;

// ---- 工具：chrome API 错误兜底 ----
// 返回 lastError.message 字符串（已解包）；调用方直接用即可，不要再 .message。
// 修复历史：早期版本返回整个 lastError 对象，调用方又取 .message，导致 undefined 显示。
function lastError() {
  try {
    if (!chrome.runtime.lastError) return null;
    return chrome.runtime.lastError.message || '(无错误详情)';
  } catch (_) { return null; }
}

// 按错误内容推断可能原因（用于自检失败时的引导文案）
// lastError.message 典型值：
//   - "Could not establish connection. Receiving end does not exist." → content script 未注入
//   - "The message port closed before a response was received."        → receiver 异步丢失
//   - "Message manager disconnected"                                   → MV3 通道关闭
function diagnoseUninjected(errMsg, tabUrl) {
  const onSupportedHost = /douyin\.com|xiaohongshu\.com|tiktok\.com/i.test(tabUrl || '');
  const lines = [];
  if (errMsg && /Could not establish connection|Receiving end does not exist|port closed|disconnected/i.test(errMsg)) {
    if (!onSupportedHost) {
      lines.push('  1. 当前 URL 不在 manifest matches 范围内（仅抖音/小红书/TikTok 网页版生效）');
    } else {
      lines.push('  1. 扩展未加载 / 被禁用（chrome://extensions 检查开关）');
      lines.push('  2. content script 尚未执行（请等待页面加载完成，或按 Ctrl+Shift+R 强制刷新一次）');
      lines.push('  3. 此标签页在扩展加载前已打开，需要重新加载页面');
      lines.push('  4. 页面在 iframe 中（content script 默认不注入 iframe）');
    }
  } else {
    lines.push('  1. 扩展未加载 / 被禁用');
    lines.push('  2. content script 尚未执行（请等待页面加载完成）');
    lines.push('  3. 页面已登录但 URL 不在 manifest matches 范围内');
  }
  return lines.join('\n');
}

function showBanner(kind, title, body) {
  const el = $('banner');
  el.className = `banner show ${kind}`;
  el.innerHTML = '';
  if (title) {
    const t = document.createElement('div');
    t.className = 'title';
    t.textContent = title;
    el.appendChild(t);
  }
  if (body) {
    const b = document.createElement('div');
    b.textContent = body;
    el.appendChild(b);
  }
}

function clearBanner() {
  $('banner').className = 'banner';
  $('banner').textContent = '';
}

// ---- 工具：URL 规范化 ----
function normalizeServerUrl(raw) {
  if (!raw) return '';
  let s = String(raw).trim();
  if (!s) return '';
  if (!/^https?:\/\//i.test(s)) s = 'http://' + s;
  // 去掉尾斜杠
  s = s.replace(/\/+$/, '');
  return s;
}

// ---- 配置存取 ----
function loadConfig(cb) {
  try {
    chrome.storage.local.get('bridgeConfig', (res) => {
      const cfg = res && res.bridgeConfig;
      cb(cfg || { serverUrl: '', token: '', autoConnect: true });
    });
  } catch (e) {
    showBanner('error', '读取配置失败', String(e));
    cb({ serverUrl: '', token: '', autoConnect: true });
  }
}

function saveConfig(cfg, cb) {
  try {
    chrome.storage.local.set({ bridgeConfig: cfg }, () => {
      const err = lastError();
      if (err) {
        showBanner('error', '保存失败', err);
        cb && cb(false);
        return;
      }
      try {
        chrome.runtime.sendMessage({ type: 'setConfig', config: cfg }, (resp) => {
          const e = lastError();
          if (e) {
            showBanner('warn', '已写入 storage，但 background 未响应', e + '（请检查扩展是否被禁用）');
            cb && cb(false);
            return;
          }
          cb && cb(resp && resp.ok);
        });
      } catch (e) {
        showBanner('warn', '已写入 storage，但 background 通信失败', String(e));
        cb && cb(false);
      }
    });
  } catch (e) {
    showBanner('error', '保存失败', String(e));
    cb && cb(false);
  }
}

// ---- 测试连接：fetch 健康检查端点 ----
// 策略：依次尝试 HEALTH_PATHS 多个候选路径
//   - 2xx：服务端可达，立即返回成功
//   - 5xx：服务端可达但降级（PG/Redis/LLM 故障），仍记为"可连"，提示用户
//   - 404：当前路径不存在，继续试下一个（避免误报）
//   - 其他 4xx：认证/权限问题，立即返回
//   - 网络错误/超时/拒绝：保留 last error，作为 unreachable 返回
async function testConnection(serverUrl) {
  const base = normalizeServerUrl(serverUrl);
  if (!base) return { ok: false, reason: 'empty' };
  let lastNetErr = null;
  for (const p of HEALTH_PATHS) {
    const url = base + p;
    const ctl = new AbortController();
    const t = setTimeout(() => ctl.abort(), POPUP_HEALTH_CHECK_TIMEOUT_MS);
    try {
      const res = await fetch(url, { method: 'GET', signal: ctl.signal, cache: 'no-store' });
      clearTimeout(t);
      if (res.ok) return { ok: true, url, status: res.status, degraded: false };
      if (res.status === 404) continue; // 路径不存在，尝试下一个候选
      if (res.status >= 500 && res.status < 600) return { ok: true, url, status: res.status, degraded: true };
      if (res.status >= 400 && res.status < 500) return { ok: false, url, status: res.status, reason: 'http_' + res.status };
      // 其他 3xx：跟随重定向或视为不可达
      return { ok: false, url, status: res.status, reason: 'http_' + res.status };
    } catch (e) {
      clearTimeout(t);
      lastNetErr = e && e.message ? e.message : String(e);
      // 网络错误：尝试下一个候选
    }
  }
  return { ok: false, url: base, reason: 'unreachable', detail: lastNetErr };
}

// ---- 连接状态 ----
function refreshStatus() {
  try {
    chrome.runtime.sendMessage({ type: 'getStatus' }, (res) => {
      const err = lastError();
      if (err) {
        $('statusOut').textContent = 'background 无响应：' + err;
        return;
      }
      if (!res) {
        $('statusOut').textContent = '无响应';
        return;
      }
      const lines = Object.entries(res.statuses || {}).map(([k, v]) => {
        const tag = v.online ? '🟢 在线' : '🔴 离线';
        return `${tag}  ${k}\n   会话: ${v.conversationId || '-'}\n   账号: ${v.accountId || '-'}\n   渠道: ${v.channel || '-'}`;
      });
      $('statusOut').textContent = lines.length ? lines.join('\n\n') : '当前无已连接账号';
    });
  } catch (e) {
    $('statusOut').textContent = '查询失败：' + e;
  }
}

// ---- 自检：向当前标签页的 content script 发 ping → selfcheck 两步走 ----
// 协议设计（与 content/common.js 对齐）：
//   1) 先 ping：内容脚本在不匹配页面也会应答，用于"是否注入"探活
//   2) ping 成功后再 selfcheck：未匹配页返回 matched=false + 提示，不报错
// 这样能区分三种情况：
//   A. ping 都失败 → content script 根本未注入（扩展禁用 / URL 不匹配 / 旧标签页）
//   B. ping 成功 + selfcheck 返回 matched=false → 内容脚本在，但当前不是私信/消息页
//   C. ping 成功 + selfcheck 返回完整数据 → 桥接健康
//
// 全自动修复：ping 失败时自动请求 background 执行 programmatic 注入，
// 解决"标签页在扩展加载/更新前已打开"场景（接收端不存在）。
// 用户无需手动刷新页面。
function selfCheck() {
  try {
    chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
      const tab = tabs && tabs[0];
      if (!tab) {
        $('selfOut').textContent = '无活动标签页';
        return;
      }
      if (!tab.url) {
        $('selfOut').textContent = '当前标签页 URL 不可读（请打开非 chrome:// 页面）';
        return;
      }
      // 第一道闸：域名白名单
      const onSupportedHost = /douyin\.com|xiaohongshu\.com|tiktok\.com/i.test(tab.url);
      if (!onSupportedHost) {
        $('selfOut').textContent = `当前标签页不是抖音/小红书/TikTok：\n${tab.url}\n\n请在对应平台网页（已登录）上点击。`;
        return;
      }
      // 第二道闸：ping 探活
      pingContentScript(tab, (pingErr) => {
        if (pingErr) {
          // ---- 自愈：尝试 programmatic 注入 ----
          // 标签页在扩展加载/更新前已打开，content script 不会自动加载。
          // 这里主动请求 background 注入，注入成功后再重试一次 ping。
          $('selfOut').textContent = `未检测到桥接，正在自动注入…`;
          tryAutoInject(tab, (injectResult) => {
            if (!injectResult.ok) {
              // 注入失败：仍展示原错误 + 原因
              $('selfOut').textContent = `该页面未注入桥接：${pingErr.msg}\n\n自动注入失败：${injectResult.reason}${injectResult.error ? '（' + injectResult.error + '）' : ''}\n\n可能原因：\n${pingErr.hints}`;
              return;
            }
            // 注入成功：等 200ms 让 content script 注册监听器，然后重试 ping
            setTimeout(() => {
              pingContentScript(tab, (retryErr) => {
                if (retryErr) {
                  $('selfOut').textContent = `自动注入 ${injectResult.file} 成功但 ping 仍失败：${retryErr.msg}\n\n请手动按 Ctrl+Shift+R 刷新一次。\n（注入后 content script 需页面重新加载才能完全生效）`;
                  return;
                }
                // 自愈成功
                $('selfOut').textContent = `✅ 已自动注入 ${injectResult.file}，桥接已恢复。\n正在拉取详细数据…`;
                runSelfcheck(tab);
              });
            }, 200);
          });
          return;
        }
        // 第三道闸：selfcheck 详细数据
        runSelfcheck(tab);
      });
    });
  } catch (e) {
    $('selfOut').textContent = '查询标签页失败：' + e;
  }
}

// 请求 background 执行 programmatic 注入
function tryAutoInject(tab, cb) {
  try {
    chrome.runtime.sendMessage(
      { type: 'injectContentScript', tabId: tab.id, url: tab.url, allFrames: false },
      (resp) => {
        const e = lastError();
        if (e) {
          cb({ ok: false, reason: 'background_no_response', error: e });
          return;
        }
        if (!resp) {
          cb({ ok: false, reason: 'empty_response' });
          return;
        }
        cb(resp);
      }
    );
  } catch (e) {
    cb({ ok: false, reason: 'send_threw', error: String(e) });
  }
}

function pingContentScript(tab, cb) {
  try {
    chrome.tabs.sendMessage(tab.id, { type: 'ping' }, (resp) => {
      const err = lastError();
      if (err) {
        // lastError 已经是字符串；err.message 必然 undefined
        cb({ msg: err, hints: diagnoseUninjected(err, tab.url) });
        return;
      }
      if (!resp || !resp.pong) {
        cb({ msg: 'content script 未响应 ping', hints: diagnoseUninjected(null, tab.url) });
        return;
      }
      cb(null);
    });
  } catch (e) {
    cb({ msg: '发送 ping 失败：' + e, hints: diagnoseUninjected(null, tab.url) });
  }
}

function runSelfcheck(tab) {
  try {
    chrome.tabs.sendMessage(tab.id, { type: 'selfcheck' }, (resp) => {
      const err = lastError();
      if (err) {
        $('selfOut').textContent = 'ping 已通过但 selfcheck 失败：' + err + '\n（content script 已注入但响应中断，请刷新页面重试）';
        return;
      }
      if (!resp) {
        $('selfOut').textContent = 'selfcheck 未返回数据';
        return;
      }
      if (resp.matched === false) {
        $('selfOut').textContent = `内容脚本已注入，但当前不是私信/消息页。\n\nURL: ${tab.url}\n频道: ${resp.channel}\n提示: ${resp.hint || '请打开目标会话的【私信/聊天】界面（不是首页/feed）后再次校准'}`;
        return;
      }
      const sel = resp.selectors ? '\n选择器:\n' + JSON.stringify(resp.selectors, null, 2) : '';
      const sample = (resp.sample || []).map((s) => `  [${s.sender}] ${s.text}`).join('\n');
      $('selfOut').textContent = `频道: ${resp.channel}\n匹配: ${resp.matched}\n账号: ${resp.accountId || '(空)'}\n会话: ${resp.conversationId || '(空)'}\n消息条目: ${resp.msgItemCount}\n样本:\n${sample}${sel}`;
    });
  } catch (e) {
    $('selfOut').textContent = '发送 selfcheck 失败：' + e;
  }
}

// ---- 打开私信页 ----
function openUrl(url) {
  try {
    chrome.tabs.create({ url });
  } catch (e) {
    showBanner('error', '打开失败', String(e));
  }
}

// ---- DOM ready ----
document.addEventListener('DOMContentLoaded', () => {
  loadConfig((cfg) => {
    $('serverUrl').value = cfg.serverUrl || '';
    $('token').value = cfg.token || '';
    if (!cfg.serverUrl) $('serverUrl').placeholder = DEFAULT_PLACEHOLDER;
  });

  // 保存按钮
  $('save').addEventListener('click', async () => {
    const btn = $('save');
    const serverUrl = normalizeServerUrl($('serverUrl').value);
    if (!serverUrl) {
      showBanner('error', '请输入服务端地址', '例如 ' + DEFAULT_PLACEHOLDER);
      $('serverUrl').focus();
      return;
    }
    btn.disabled = true;
    const cfg = { serverUrl, token: $('token').value.trim(), autoConnect: true };
    saveConfig(cfg, (ok) => {
      btn.disabled = false;
      if (ok) {
        showBanner('success', '✓ 已保存', '配置已写入 Chrome storage，background 收到。新打开抖音/小红书/TikTok 私信页即生效。');
      }
      refreshStatus();
    });
  });

  // 测试连接
  $('test').addEventListener('click', async () => {
    const btn = $('test');
    const serverUrl = normalizeServerUrl($('serverUrl').value);
    if (!serverUrl) {
      showBanner('error', '请输入服务端地址', '例如 ' + DEFAULT_PLACEHOLDER);
      return;
    }
    btn.disabled = true;
    showBanner('info', '⏳ 正在测试…', `${serverUrl} /api/health`);
    const r = await testConnection(serverUrl);
    btn.disabled = false;
    if (r.ok && r.degraded) {
      showBanner('warn', '⚠ 服务端可达但已降级', `${r.url}  HTTP ${r.status}\n\nPG/Redis/LLM 依赖可能故障。桥接仍可工作，但 AI 回复会变慢或失败。`);
    } else if (r.ok) {
      showBanner('success', '✓ 服务端可达', `${r.url}  HTTP ${r.status}\n\n请打开 抖音/小红书/TikTok 私信页启动桥接。`);
    } else if (r.reason === 'empty') {
      showBanner('error', '请输入服务端地址', '例如 ' + DEFAULT_PLACEHOLDER);
    } else if (r.reason && /^http_/.test(r.reason)) {
      showBanner('warn', '⚠ 服务端响应 4xx', `${r.url}  ${r.reason}\n\n检查：\n  1. user-server 是否在运行\n  2. 健康检查路径是否被中间件拦截\n  3. URL 是否正确`);
    } else {
      const detail = r.detail ? `\n详细: ${r.detail}` : '';
      showBanner('error', '✗ 无法连接', `${r.url}${detail}\n\n可能原因：\n  1. user-server 未启动（默认端口 ${DEFAULT_PORT_HINT}）\n  2. 端口不正确：检查 cmd/api/main.go 或 PORT 环境变量\n  3. 防火墙/CORS 拦截\n  4. URL 写错\n  5. manifest.json host_permissions 未覆盖此域名`);
    }
  });

  // 回车提交
  $('serverUrl').addEventListener('keydown', (e) => { if (e.key === 'Enter') $('save').click(); });
  $('token').addEventListener('keydown', (e) => { if (e.key === 'Enter') $('save').click(); });

  // 状态/自检
  $('status').addEventListener('click', refreshStatus);
  $('selfcheck').addEventListener('click', selfCheck);

  // 打开私信页快捷入口（URL 来源：constants.js PLATFORM_ENTRY_URLS）
  $('openDouyin').addEventListener('click', () => openUrl(PLATFORM_ENTRY_URLS.douyin_web));
  $('openXhs').addEventListener('click', () => openUrl(PLATFORM_ENTRY_URLS.xhs_web));
  $('openTiktok').addEventListener('click', () => openUrl(PLATFORM_ENTRY_URLS.tiktok_web));

  // 首次打开时拉一次状态
  refreshStatus();
});

// 暴露到全局便于单测
if (typeof window !== 'undefined') {
  window.__popup = {
    normalizeServerUrl,
    testConnection,
    saveConfig,
    loadConfig,
    showBanner,
    clearBanner,
    lastError,
    diagnoseUninjected,
    selfCheck,
    pingContentScript,
    runSelfcheck,
    tryAutoInject,
  };
}
