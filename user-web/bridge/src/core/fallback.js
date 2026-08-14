// 账号/会话判定兜底工具
//
// 8: account_id 多层 fallback 派生
//   同一平台可能因登录态/页面结构不同，URL 路径、用户链接、侧边栏头像均可能为空。
//   派生顺序：URL path > 用户链接 > meta 标签 > 持久化缓存(chrome.storage) > 默认值

const ACCOUNT_CACHE_KEY = 'bridgeAccountFallback';

// 8 account_id 多层 fallback 派生 ——
// 2026-08-14 治本：失败时返回空串（不污染为 `${channel}-unknown`）。
// 背景：前端 getAccountId() DOM 多层兜底（URL 路径/用户链接/meta 标签/缓存）全部失败时，
// 旧逻辑回退 `${channel}-unknown`（如 `douyin-unknown`），污染后端入库与按 account_id 关联
// outbound 的查询链路，导致 AI 出站回采识别失败、回环拦截失效。
// 改为空串后，后端层0 改用 (platform + sender_name=右侧header + content) 三元组命中 outbound，
// 完全不依赖 account_id；account_id 缺失不应被用于关联判据，避免被未知占位污染。
export function deriveAccountId(channel, candidates) {
  for (const c of candidates) {
    if (c && typeof c === 'string' && c.trim()) return c.trim();
  }
  return '';
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
  if (primary) {
    setCachedAccount(channel, primary);
    return primary;
  }
  // 2) 缓存（同一账号即使链接暂时取不到，仍可恢复）
  const cached = await getCachedAccount(channel);
  if (cached) return cached;
  // 3) 兜底：返回空串（不污染为 `${channel}-unknown`，2026-08-14 治本）
  return '';
}

// 前端统一自/他占位：自/他判定已完全移交后端（服务端内容回显检测 + sender_type 强制覆盖），
// 前端不再计算 self/other。非 system/recall 消息一律填 customer 占位，后端会强制覆盖；
// 仅 system/recall 需前端识别（消息类型，否则后端误触发 AI）。
export const FRONTEND_DEFAULT_SENDER_TYPE = 'customer';
