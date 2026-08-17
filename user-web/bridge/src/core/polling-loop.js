/**
 * PollingLoop —— 下发轮询调度器
 *
 * 巡检（遍历会话列表）由 ChannelAdapter.startPatrol 自循环负责
 * （PATROL_DEFAULTS 风控节奏：30s 间隔、maxPerRound=6、3-5s 切换、会话冷却 120s）。
 * 本类只负责 downlink outbox 下发轮询，避免双套巡检导致 0ms 空转。
 */
import { createLogger } from './logger.js';
import { BRIDGE_THREE_CHANNEL } from './constants.js';
import { pollDownlink, initDownlink } from './downlink.js';

const log = createLogger('bridge', 'bg');

export class PollingLoop {
  constructor({ channels, getConfig, getMeta, getAdapter, uplinks }) {
    this.channels = channels || [];
    this.getConfig = getConfig;
    this.getMeta = getMeta;
    this.getAdapter = getAdapter;
    this.uplinks = uplinks || new Map();
    this._running = false;
    this._downlinkTimer = null;
    this._downlinkInFlight = false;
  }

  start() {
    if (this._running) return;
    this._running = true;
    this._downlinkTimer = setInterval(
      () => this._downlinkSafe(),
      BRIDGE_THREE_CHANNEL.outboxPollIntervalMs
    );
    initDownlink(this.channels);
    log.info(`下发轮询已启动：每 ${BRIDGE_THREE_CHANNEL.outboxPollIntervalMs}ms`);
  }

  stop() {
    this._running = false;
    if (this._downlinkTimer) clearInterval(this._downlinkTimer);
    this._downlinkTimer = null;
    log.info('下发轮询已停止');
  }

  async _downlinkSafe() {
    if (!this._running || this._downlinkInFlight) return;
    this._downlinkInFlight = true;
    try {
      await this._downlink();
    } catch (e) {
      log.error('下发轮询失败', e);
    } finally {
      this._downlinkInFlight = false;
    }
  }

  async _downlink() {
    const cfg = await this.getConfig();
    if (!cfg) return;
    const meta = this.getMeta ? this.getMeta() : {};
    const accountId = (meta && meta.accountId) || (cfg.active && cfg.active.accountId) || 'default';
    for (const ch of this.channels) {
      try {
        const adapter = this.getAdapter ? this.getAdapter(ch) : null;
        const sendOutbound = adapter && typeof adapter.sendOutbound === 'function'
          ? (text, convId, opts) => adapter.sendOutbound(text, convId, opts)
          : undefined;
        await pollDownlink(ch, accountId, () => this.getConfig(), {
          sendOutbound,
          sendOutboundTimeoutMs: BRIDGE_THREE_CHANNEL.sendOutboundTimeoutMs,
          outboxBatchSize: BRIDGE_THREE_CHANNEL.outboxBatchSize,
        });
      } catch (e) {
        log.error(`下发轮询 [${ch}] 异常`, e);
      }
    }
  }
}
