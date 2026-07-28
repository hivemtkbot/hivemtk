// popup 逻辑：配置后端地址/鉴权、测试连接、查看连接状态、自检私信页。
//
// 设计要点：
//   - 状态用 banner（持久可见）而非按钮文字闪烁；不再让用户看不到反馈
//   - 所有 chrome.runtime.sendMessage / tabs.sendMessage 都有 lastError 兜底
//   - "测试连接"：fetch /api/health 验证 URL 真实可达
//   - "打开私信页"：直接开新标签，免去用户手动导航
//   - 输入框支持回车提交
const $ = (id) => document.getElementById(id);

const DEFAULT_PORT_HINT = 8204;
const DEFAULT_PLACEHOLDER = `http://localhost:${DEFAULT_PORT_HINT}`;
// user-server 健康检查端点（router.go 实际注册 /health /healthz /readyz）
// 优先 /health（含依赖检查，2xx=健康，503=可连但降级），其次存活探针。
const HEALTH_PATHS = ['/health', '/healthz', '/readyz', '/api/health'];

// ---- 工具：chrome API 错误兜底 ----
function lastError() {
  try { return chrome.runtime.lastError ? chrome.runtime.lastError.message : null; } catch (_) { return null; }
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
    const t = setTimeout(() => ctl.abort(), 3500);
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

// ---- 自检：向当前标签页的 content script 发 selfcheck ----
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
      if (!/douyin\.com|xiaohongshu\.com|tiktok\.com/i.test(tab.url)) {
        $('selfOut').textContent = `当前标签页不是私信页：\n${tab.url}\n\n请打开 抖音 / 小红书 / TikTok 私信页（已登录状态）后点击。`;
        return;
      }
      try {
        chrome.tabs.sendMessage(tab.id, { type: 'selfcheck' }, (resp) => {
          const err = lastError();
          if (err) {
            $('selfOut').textContent = '该页面未注入桥接：' + err.message + '\n\n可能原因：\n  1. 扩展未加载 / 被禁用\n  2. content script 尚未执行（请等待页面加载完成）\n  3. 页面已登录但 URL 不在 manifest matches 范围内';
            return;
          }
          if (!resp) {
            $('selfOut').textContent = '该页面未注入桥接（content script 未响应）';
            return;
          }
          const sel = resp.selectors ? '\n选择器:\n' + JSON.stringify(resp.selectors, null, 2) : '';
          const sample = (resp.sample || []).map((s) => `  [${s.sender}] ${s.text}`).join('\n');
          $('selfOut').textContent = `频道: ${resp.channel}\n匹配: ${resp.matched}\n账号: ${resp.accountId || '(空)'}\n会话: ${resp.conversationId || '(空)'}\n消息条目: ${resp.msgItemCount}\n样本:\n${sample}${sel}`;
        });
      } catch (e) {
        $('selfOut').textContent = '发送自检消息失败：' + e;
      }
    });
  } catch (e) {
    $('selfOut').textContent = '查询标签页失败：' + e;
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

  // 打开私信页快捷入口
  $('openDouyin').addEventListener('click', () => openUrl('https://www.douyin.com/'));
  $('openXhs').addEventListener('click', () => openUrl('https://www.xiaohongshu.com/'));
  $('openTiktok').addEventListener('click', () => openUrl('https://www.tiktok.com/'));

  // 首次打开时拉一次状态
  refreshStatus();
});

// 暴露到全局便于单测
if (typeof window !== 'undefined') {
  window.__popup = { normalizeServerUrl, testConnection, saveConfig, loadConfig, showBanner, clearBanner };
}
