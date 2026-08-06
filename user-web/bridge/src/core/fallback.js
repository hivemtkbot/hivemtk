// 账号/会话判定兜底工具
//
// 8: account_id 多层 fallback 派生
//   同一平台可能因登录态/页面结构不同，URL 路径、用户链接、侧边栏头像均可能为空。
//   派生顺序：URL path > 用户链接 > meta 标签 > 持久化缓存(chrome.storage) > 默认值

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

// 前端统一自/他占位：自/他判定已完全移交后端（基于内容 hash + DB 读取）权威处理，
// 前端不再计算 self/other（见 MEMORY）。非 system/recall 消息一律填 customer 占位，
// 后端会强制覆盖；仅 system/recall 需前端识别（消息类型，否则后端误触发 AI）。
export const FRONTEND_DEFAULT_SENDER_TYPE = 'customer';
