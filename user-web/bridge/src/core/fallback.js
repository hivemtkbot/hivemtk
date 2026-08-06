// 账号/会话/自他判定兜底工具
//
// 8: account_id 多层 fallback 派生
//   同一平台可能因登录态/页面结构不同，URL 路径、用户链接、侧边栏头像均可能为空。
//   派生顺序：URL path > 用户链接 > meta 标签 > 持久化缓存(chrome.storage) > 默认值
//
// 9: 自他消息判定兜底（头像位置）
//   抖音/小红书 IM 自他气泡通常有 .right/.left class，但部分版本未必。
//   兜底：检查消息项内 [class*="avatar"] 的 right 值或 grid-column-start

const ACCOUNT_CACHE_KEY = 'bridgeAccountFallback';

// 8 account_id 多层 fallback 派生 ——
export function deriveAccountId(channel, candidates) {
  for (const c of candidates) {
    if (c && typeof c === 'string' && c.trim()) return c.trim();
  }
  // 兜底：返回带时间戳的 stable 字符串（不会再次变化，便于会话内一致）
  return `${channel}-unknown`;
}

export async function getCachedAccount(channel) {
  try {
    return new Promise((resolve) => {
      chrome.storage.local.get([ACCOUNT_CACHE_KEY], (res) => {
        const m = res && res[ACCOUNT_CACHE_KEY];
        if (m && typeof m === 'object' && m[channel]) return resolve(m[channel]);
        resolve(null);
      });
    });
  } catch (e) {
    return null;
  }
}

export function setCachedAccount(channel, accountId) {
  if (!accountId) return;
  try {
    chrome.storage.local.get([ACCOUNT_CACHE_KEY], (res) => {
      const m = (res && res[ACCOUNT_CACHE_KEY]) || {};
      m[channel] = accountId;
      chrome.storage.local.set({ [ACCOUNT_CACHE_KEY]: m });
    });
  } catch (e) {
    // storage 不可用时静默忽略
  }
}

export async function resolveAccountIdWithFallback(channel, primaryCandidates) {
  // 1) 主候选列表（按渠道 hooks 给出的 URL 链接、侧边栏链接等）
  const primary = deriveAccountId(channel, primaryCandidates);
  if (!primary.endsWith('-unknown')) {
    setCachedAccount(channel, primary);
    return primary;
  }
  // 2) 缓存（同一账号即使链接暂时取不到，仍可恢复）
  const cached = await getCachedAccount(channel);
  if (cached) return cached;
  // 3) 兜底：返回 unknown
  return primary;
}

// 9 自他消息判定兜底 ——
// 漏斗顺序：显式 self 命中 → 显式 other 命中（互斥排除） → 气泡整体对齐 → 头像位置 → data 属性
// ⚠️ 任一选择器匹配路径必须同时覆盖：item 自身 (matches) / 祖先 (closest) / 子元素 (querySelector)
//    因为平台真实结构中 self/other 标记 class 可能在 item 的子节点上（如 xhs 的 .chat-item__content--right）。
export function isSelfMessage(item, explicitSelfSelector, explicitOtherSelector) {
  if (!item) return false;
  // 任意 selector + 任意匹配方向（自身/祖先/子元素）是否命中
  const _anyHit = (sel) => {
    if (!sel) return false;
    try {
      if (item.matches && item.matches(sel)) return true;
    } catch (_) { /* 非法选择器忽略 */ }
    try {
      if (item.closest && item.closest(sel)) return true;
    } catch (_) { /* 非法选择器忽略 */ }
    try {
      if (item.querySelector && item.querySelector(sel)) return true;
    } catch (_) { /* 非法选择器忽略 */ }
    return false;
  };
  // 1) 显式 self 命中（最高优先级）
  if (explicitSelfSelector && _anyHit(explicitSelfSelector)) return true;
  // 1.5) 显式 other 命中：互斥排除（命中对方标记就不可能是自己消息）
  if (explicitOtherSelector && _anyHit(explicitOtherSelector)) return false;
  // 2) 气泡整体水平对齐：气泡相对其最近滚动父/消息容器的位置
  //    —— 自方气泡整体贴右侧，气泡.left 越过容器水平中线；对方贴左侧。
  //    ⚠️ 修复（小红书回环根因）：小红书等平台 .chat-item 是「整行容器」（横跨列表全宽），
  //       真正的气泡是内部 .chat-item__content--right / --left。若直接测量整行，
  //       item.left≈list.left、item.right≈list.right → 两侧都不越过中线 → 兜底失效 → 误判「他人」
  //       → 卖家自己的 AI 回复被当成客户消息上报 → 后端不断触发 AI → 平台不断下发。
  //       故先定位内部气泡元素，测量其真实几何位置（右侧=自己，左侧=他人）。
  try {
    const bubble =
      item.querySelector('[class*="content--right" i], [class*="content--left" i], [class*="bubble" i], [class*="chat-item__content" i]')
      || item.querySelector('[class*="msg-content" i], [class*="text-message" i]')
      || item;
    const scrollRoot =
      (item.closest && (
        item.closest('[class*="msg-list" i], [class*="message-list" i], [class*="chat-content" i], [class*="chat-main" i], [class*="im-chat-window" i]')
      )) || item.parentElement || document.body;
    const listRect = scrollRoot.getBoundingClientRect();
    const itemRect = bubble.getBoundingClientRect();
    if (listRect.width > 0 && itemRect.width > 0) {
      const listCenter = listRect.left + listRect.width / 2;
      const itemLeft = itemRect.left;
      const itemRight = itemRect.right;
      // 气泡整体在容器中线右侧 → self；整体在左侧 → customer（非 self）
      if (itemLeft >= listCenter) return true;
      if (itemRight <= listCenter) return false;
      // 未越过中线（居中气泡，如系统消息）：走下一层判定
    }
  } catch (_) { /* getBoundingClientRect 不可用时忽略 */ }
  // 3) 头像位置兜底：自方头像通常在右侧
  //    —— 找第一个 avatar 元素，其水平中心超过 item 中线 → self
  try {
    const avatar = item.querySelector('[class*="avatar" i]');
    if (avatar) {
      const itemRect = item.getBoundingClientRect();
      const avRect = avatar.getBoundingClientRect();
      if (itemRect.width > 0) {
        const avCenter = avRect.left + avRect.width / 2;
        return avCenter > itemRect.left + itemRect.width / 2;
      }
    }
  } catch (_) { /* noop */ }
  // 4) data 属性兜底
  if (item.dataset && (item.dataset.sender === 'self' || item.dataset.isSelf === 'true')) {
    return true;
  }
  if (item.dataset && (item.dataset.sender === 'other' || item.dataset.isSelf === 'false')) {
    return false;
  }
  // 5) 默认 false（避免误把自己消息上报给 AI 触发回环）
  return false;
}
