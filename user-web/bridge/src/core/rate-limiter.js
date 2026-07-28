// 限速器 + 风控（防封号）
//
// 设计动机（详见 bridge.md §17.3）：平台对自动化回复极敏感，盲目高频回写会触发风控。
// 策略分三层，全部在“回写网页（最贴近平台）之前”拦截：
//   1) 拟人节奏：发送前随机等待 jitter，并强制任意两次下行的最小间隔 minIntervalMs。
//   2) Token 桶：单账号每分钟容量上限；单会话每小时上限。
//   3) 防回环/去重：同会话冷却期内不重复回复；相同文案去重窗口内不重复发送。
//
// 所有上限均为“软失败”：超限时本次下行被丢弃并记录原因，等待下次调度，绝不堆积重试。

import { RATE_LIMIT_DEFAULTS as RATE_LIMIT_DEFAULTS_FALLBACK } from './types.js';

class TokenBucket {
  constructor(capacity, refillPerMin) {
    this.capacity = capacity;
    this.refillPerMs = refillPerMin / 60000;
    this.tokens = capacity;
    this.last = Date.now();
  }
  tryAcquire(n = 1) {
    const now = Date.now();
    this.tokens = Math.min(this.capacity, this.tokens + (now - this.last) * this.refillPerMs);
    this.last = now;
    if (this.tokens >= n) {
      this.tokens -= n;
      return true;
    }
    return false;
  }
  // 预计多少毫秒后至少有 1 个 token
  retryAfterMs() {
    if (this.tokens >= 1) return 0;
    return Math.ceil((1 - this.tokens) / this.refillPerMs);
  }
}

function hashStr(s) {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (Math.imul(31, h) + s.charCodeAt(i)) | 0;
  return (h >>> 0).toString(36);
}

export class RateLimiter {
  constructor(cfg = {}) {
    this.cfg = { ...RATE_LIMIT_DEFAULTS_FALLBACK, ...cfg };
    this.accountBuckets = new Map();   // channel:account -> TokenBucket
    this.convBuckets = new Map();      // channel:account:conv -> {bucket, hourly:[ts...], lastSentAt, lastHash}
    this.lastGlobalSendAt = 0;
  }

  _accountKey(channel, account) { return `${channel}:${account}`; }
  _convKey(channel, account, conv) { return `${channel}:${account}:${conv || '_'}`; }

  _accountBucket(channel, account) {
    const k = this._accountKey(channel, account);
    if (!this.accountBuckets.has(k)) {
      this.accountBuckets.set(k, new TokenBucket(this.cfg.accountCapacity, this.cfg.accountRefillPerMin));
    }
    return this.accountBuckets.get(k);
  }

  _convState(channel, account, conv) {
    const k = this._convKey(channel, account, conv);
    if (!this.convBuckets.has(k)) {
      this.convBuckets.set(k, {
        bucket: new TokenBucket(this.cfg.conversationPerHour, this.cfg.conversationPerHour),
        hourly: [],
        lastSentAt: 0,
        lastHash: '',
        lastHashAt: 0,
      });
    }
    return this.convBuckets.get(k);
  }

  // 尝试获取一次下行许可。返回 { allowed, reason, retryAfterMs, waitHintMs }
  // waitHintMs：建议发送前等待的拟人延迟（jitter + 最小间隔），调用方 await 后再真正回写。
  tryAcquire(channel, account, conversation, text) {
    const now = Date.now();
    const reasons = [];

    // —— 层1：拟人节奏（最小全局间隔）——
    const sinceLast = now - this.lastGlobalSendAt;
    if (sinceLast < this.cfg.minIntervalMs) {
      reasons.push(`minInterval ${this.cfg.minIntervalMs}ms`);
    }

    // —— 层2a：单账号 Token 桶 ——
    const ab = this._accountBucket(channel, account);
    const accountOk = ab.tryAcquire(1);
    if (!accountOk) reasons.push(`account bucket(${this.cfg.accountCapacity}/min)`);

    // —— 层2b：单会话每小时桶 ——
    const cs = this._convState(channel, account, conversation);
    cs.hourly = cs.hourly.filter((t) => now - t < 3600000);
    const convOk = cs.bucket.tryAcquire(1);
    if (!convOk || cs.hourly.length >= this.cfg.conversationPerHour) {
      reasons.push(`conversation ${this.cfg.conversationPerHour}/hour`);
    }

    // —— 层3a：会话冷却（防回环/刷屏）——
    const cooldownOk = now - cs.lastSentAt >= this.cfg.conversationCooldownMs;
    if (!cooldownOk) reasons.push(`cooldown ${this.cfg.conversationCooldownMs}ms`);

    // —— 层3b：相同文案去重 ——
    let dedupOk = true;
    if (text) {
      const h = hashStr(text);
      if (h === cs.lastHash && now - cs.lastHashAt < this.cfg.dedupWindowMs) {
        dedupOk = false;
        reasons.push('dedup same text');
      }
    }

    if (reasons.length || !accountOk || !convOk || !cooldownOk || !dedupOk) {
      // 退款已扣的桶（本次未真正发送）
      if (accountOk) ab.tokens = Math.min(ab.capacity, ab.tokens + 1);
      if (convOk) cs.bucket.tokens = Math.min(cs.bucket.capacity, cs.bucket.tokens + 1);
      return {
        allowed: false,
        reason: reasons.join('; ') || 'blocked',
        retryAfterMs: ab.retryAfterMs(),
        waitHintMs: 0,
      };
    }

    // 计算拟人延迟（jitter + 补足最小间隔）
    const waitHintMs = Math.max(
      this.cfg.minIntervalMs - sinceLast,
      this.cfg.jitterMinMs + Math.random() * (this.cfg.jitterMaxMs - this.cfg.jitterMinMs)
    );
    return { allowed: true, reason: 'ok', retryAfterMs: 0, waitHintMs: Math.round(waitHintMs) };
  }

  // 真正发送成功后登记（调用方在回写网页成功后调用）
  markSent(channel, account, conversation, text) {
    const now = Date.now();
    this.lastGlobalSendAt = now;
    const cs = this._convState(channel, account, conversation);
    cs.lastSentAt = now;
    cs.hourly.push(now);
    if (text) {
      cs.lastHash = hashStr(text);
      cs.lastHashAt = now;
    }
  }
}
