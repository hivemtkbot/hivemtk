// 账号/会话/自他判定兜底工具
//
// P2-S2-8: account_id 多层 fallback 派生
//   同一平台可能因登录态/页面结构不同，URL 路径、用户链接、侧边栏头像均可能为空。
//   派生顺序：URL path > 用户链接 > meta 标签 > 持久化缓存(chrome.storage) > 默认值
//
// P2-S2-9: 自他消息判定兜底（头像位置）
//   抖音/小红书 IM 自他气泡通常有 .right/.left class，但部分版本未必。
//   兜底：检查消息项内 [class*="avatar"] 的 right 值或 grid-column-start

const ACCOUNT_CACHE_KEY = 'bridgeAccountFallback';

// —— P2-S2-8 account_id 多层 fallback 派生 ——
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

// —— P2-S2-9 自他消息判定兜底 ——
export function isSelfMessage(item, explicitSelfSelector, explicitOtherSelector) {
  if (!item) return false;
  // 1) 显式 class 选择器
  if (explicitSelfSelector) {
    try {
      if (item.matches && item.matches(explicitSelfSelector)) return true;
      if (item.querySelector && item.querySelector(explicitSelfSelector)) return true;
    } catch (e) {
      // selector 非法时静默忽略
    }
  }
  // 2) 头像位置兜底：自方头像通常在右侧
  //   - 通过 item 内 [class*="avatar"] 元素的 getBoundingClientRect().left 与 item 的 rect 对比
  //   - 简单实现：找第一个 avatar 元素，left > 容器中线则判 self
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
  } catch (e) {
    // noop
  }
  // 3) data 属性兜底
  if (item.dataset && (item.dataset.sender === 'self' || item.dataset.isSelf === 'true')) {
    return true;
  }
  // 4) 默认 false（避免误把自己消息上报给 AI 触发回环）
  return false;
}
