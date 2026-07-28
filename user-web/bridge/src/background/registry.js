// 连接注册表：每个 (channel, account) 维护一条到服务端的 WebSocket。
import { BridgeClient } from '../core/bridge-client.js';
import { createLogger } from '../core/logger.js';

const log = createLogger('registry', 'bg');

class ConnectionRegistry {
  constructor() {
    this.map = new Map();
  }

  key(channel, account) {
    return `${channel}:${account || 'unknown'}`;
  }

  ensure(opts) {
    const k = this.key(opts.channel, opts.accountId);
    let conn = this.map.get(k);
    if (!conn) {
      log.info('新建服务端连接', k);
      conn = new BridgeClient(opts);
      this.map.set(k, conn);
      conn.connect();
    } else {
      // 刷新 token / conversation
      conn.token = opts.token;
      conn.conversationId = opts.conversationId;
    }
    return conn;
  }

  get(channel, account) {
    return this.map.get(this.key(channel, account));
  }

  sendHistory(channel, account, message) {
    const conn = this.get(channel, account);
    if (conn) return conn.sendHistory(message);
    return false;
  }

  remove(channel, account) {
    const conn = this.get(channel, account);
    if (conn) conn.close();
    this.map.delete(this.key(channel, account));
  }
}

export const registry = new ConnectionRegistry();
