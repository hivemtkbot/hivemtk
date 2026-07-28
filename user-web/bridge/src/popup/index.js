// popup 逻辑：配置后端地址/鉴权、查看连接状态、对当前私信页做自检（真机校准）。
const $ = (id) => document.getElementById(id);

function loadConfig(cb) {
  chrome.storage.local.get('bridgeConfig', (res) => {
    cb(res.bridgeConfig || { serverUrl: 'http://localhost:8080', token: '', autoConnect: true });
  });
}

function saveConfig(cfg, cb) {
  chrome.storage.local.set({ bridgeConfig: cfg }, () => {
    chrome.runtime.sendMessage({ type: 'setConfig', config: cfg }, cb);
  });
}

function refreshStatus() {
  chrome.runtime.sendMessage({ type: 'getStatus' }, (res) => {
    if (!res) return ($('statusOut').textContent = '无响应');
    const lines = Object.entries(res.statuses || {}).map(([k, v]) => {
      const cls = v.online ? 'status-on' : 'status-off';
      return `[${v.online ? '在线' : '离线'}] ${k} 会话:${v.conversationId || '-'} 账号:${v.accountId || '-'}`;
    });
    $('statusOut').innerHTML = lines.length
      ? lines.map((l) => `<span class="${l.includes('在线') ? 'status-on' : 'status-off'}">${l}</span>`).join('\n')
      : '当前无已连接账号';
  });
}

function selfCheck() {
  chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
    const tab = tabs[0];
    if (!tab) return ($('selfOut').textContent = '无活动标签页');
    chrome.tabs.sendMessage(tab.id, { type: 'selfcheck' }, (resp) => {
      if (chrome.runtime.lastError || !resp) {
        return ($('selfOut').textContent = '该页面未注入桥接（请确认在抖音/小红书/TikTok 私信页，且扩展已加载）');
      }
      const sel = resp.selectors ? '\n选择器:\n' + JSON.stringify(resp.selectors, null, 1) : '';
      const sample = (resp.sample || [])
        .map((s) => `  [${s.sender}] ${s.text}`)
        .join('\n');
      $('selfOut').textContent =
        `频道: ${resp.channel}\n匹配: ${resp.matched}\n账号: ${resp.accountId}\n会话: ${resp.conversationId}\n消息条目: ${resp.msgItemCount}\n样本:\n${sample}${sel}`;
    });
  });
}

document.addEventListener('DOMContentLoaded', () => {
  loadConfig((cfg) => {
    $('serverUrl').value = cfg.serverUrl || '';
    $('token').value = cfg.token || '';
  });
  $('save').addEventListener('click', () => {
    const cfg = { serverUrl: $('serverUrl').value.trim(), token: $('token').value.trim(), autoConnect: true };
    saveConfig(cfg, () => {
      $('save').textContent = '已保存';
      setTimeout(() => ($('save').textContent = '保存并连接'), 1500);
      refreshStatus();
    });
  });
  $('status').addEventListener('click', refreshStatus);
  $('selfcheck').addEventListener('click', selfCheck);
  refreshStatus();
});
