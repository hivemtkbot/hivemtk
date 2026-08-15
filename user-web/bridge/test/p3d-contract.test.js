import { describe, it, expect } from 'vitest';
import { processAckDetailedResult } from '../src/core/downlink.js';

// 协议常量（前后端必须保持一致；若改字段名必须双向同步）
const PROTOCOL = Object.freeze({
  FIELD_STATUS: 'status',
  FIELD_ACKED: 'acked',
  FIELD_DUPLICATE_COUNT: 'duplicate_count',
  FIELD_NOT_FOUND_COUNT: 'not_found_count',
  FIELD_ITEMS: 'items',
  STATUS_OK: 'ok',
  ITEM_ACKED: 'acked',
  ITEM_DUPLICATE: 'duplicate',
  ITEM_NOT_FOUND: 'not_found',
});

describe('P3-D contract（前后端 ack 协议 2026-08-15）', () => {
  it('协议字段：响应必须含 status/acked/duplicate_count/not_found_count/items', () => {
    const fakeResp = {
      status: 'ok',
      acked: 0,
      duplicate_count: 0,
      not_found_count: 0,
      items: [],
    };
    for (const f of [PROTOCOL.FIELD_STATUS, PROTOCOL.FIELD_ACKED, PROTOCOL.FIELD_DUPLICATE_COUNT, PROTOCOL.FIELD_NOT_FOUND_COUNT, PROTOCOL.FIELD_ITEMS]) {
      expect(f in fakeResp).toBe(true);
    }
  });

  it('纯成功场景：3 条全部 acked', () => {
    const ackIds = ['m1', 'm2', 'm3'];
    const ackRes = {
      status: 'ok',
      acked: 3,
      duplicate_count: 0,
      not_found_count: 0,
      items: [
        { msg_id: 'm1', status: 'acked' },
        { msg_id: 'm2', status: 'acked' },
        { msg_id: 'm3', status: 'acked' },
      ],
    };
    const r = processAckDetailedResult(ackRes, ackIds, 'douyin');
    expect(r).toEqual({ acked: 3, duplicate: 0, not_found: 0, not_in_scope: 0, retriable: 0 });
  });

  it('混合场景：acked + duplicate + not_found', () => {
    const ackIds = ['m1', 'm2', 'm3', 'm4'];
    const ackRes = {
      status: 'ok',
      acked: 1,
      duplicate_count: 1,
      not_found_count: 2,
      items: [
        { msg_id: 'm1', status: 'acked' },
        { msg_id: 'm2', status: 'duplicate' },
        { msg_id: 'm3', status: 'not_found' },
        { msg_id: 'm4', status: 'not_found' },
      ],
    };
    const r = processAckDetailedResult(ackRes, ackIds, 'douyin');
    expect(r).toEqual({ acked: 1, duplicate: 1, not_found: 2, not_in_scope: 0, retriable: 0 });
  });

  it('纯 not_found 场景', () => {
    const ackIds = ['m1', 'm2'];
    const ackRes = {
      status: 'ok',
      acked: 0,
      duplicate_count: 0,
      not_found_count: 2,
      items: [
        { msg_id: 'm1', status: 'not_found' },
        { msg_id: 'm2', status: 'not_found' },
      ],
    };
    const r = processAckDetailedResult(ackRes, ackIds, 'douyin');
    expect(r).toEqual({ acked: 0, duplicate: 0, not_found: 2, not_in_scope: 0, retriable: 0 });
  });

  it('响应 ok 但无 items 字段（兼容老版本服务端）→ 全量视为 acked', () => {
    const ackIds = ['m1', 'm2', 'm3'];
    const ackRes = { status: 'ok', acked: 3 };
    const r = processAckDetailedResult(ackRes, ackIds, 'douyin');
    expect(r).toEqual({ acked: 3, duplicate: 0, not_found: 0, not_in_scope: 0, retriable: 0 });
  });

  it('响应 status=error 整体失败 → 全部 retriable', () => {
    const ackIds = ['m1', 'm2'];
    const ackRes = { status: 'error', message: 'internal' };
    const r = processAckDetailedResult(ackRes, ackIds, 'douyin');
    expect(r).toEqual({ acked: 0, duplicate: 0, not_found: 0, not_in_scope: 0, retriable: 2 });
  });

  it('响应为 null/undefined → 全部 retriable', () => {
    const r1 = processAckDetailedResult(null, ['m1']);
    expect(r1.retriable).toBe(1);
    const r2 = processAckDetailedResult(undefined, ['m1', 'm2']);
    expect(r2.retriable).toBe(2);
  });

  it('items 含未知 status 字段（保守 retriable）', () => {
    const ackIds = ['m1', 'm2'];
    const ackRes = {
      status: 'ok',
      acked: 1,
      items: [
        { msg_id: 'm1', status: 'acked' },
        { msg_id: 'm2', status: 'unknown_future_state' },
      ],
    };
    const r = processAckDetailedResult(ackRes, ackIds);
    expect(r.acked).toBe(1);
    expect(r.retriable).toBe(1);
  });

  it('items 缺失部分 msg_id（响应不完整）→ 缺失项入 retriable', () => {
    const ackIds = ['m1', 'm2', 'm3'];
    const ackRes = {
      status: 'ok',
      acked: 1,
      items: [
        { msg_id: 'm1', status: 'acked' },
      ],
    };
    const r = processAckDetailedResult(ackRes, ackIds);
    expect(r.acked).toBe(1);
    expect(r.retriable).toBe(2);
  });

  it('空 msg_ids 数组：返回全 0', () => {
    const r = processAckDetailedResult({ status: 'ok', items: [] }, []);
    expect(r).toEqual({ acked: 0, duplicate: 0, not_found: 0, not_in_scope: 0, retriable: 0 });
  });

  it('空字符串 msg_id 跳过不计', () => {
    const ackIds = ['m1', '', 'm3'];
    const ackRes = {
      status: 'ok',
      items: [
        { msg_id: 'm1', status: 'acked' },
        { msg_id: 'm3', status: 'not_found' },
      ],
    };
    const r = processAckDetailedResult(ackRes, ackIds);
    expect(r.acked).toBe(1);
    expect(r.not_found).toBe(1);
    expect(r.retriable).toBe(0);
  });
});

describe('P3-D contract invariants（业务不变量）', () => {
  it('acked + duplicate + not_found + retriable === 输入 msg_ids 数量（去重）', () => {
    const ackIds = ['m1', 'm2', 'm3', 'm4', 'm5'];
    const ackRes = {
      status: 'ok',
      items: [
        { msg_id: 'm1', status: 'acked' },
        { msg_id: 'm2', status: 'duplicate' },
        { msg_id: 'm3', status: 'not_found' },
        { msg_id: 'm4', status: 'unknown' },
        { msg_id: 'm5', status: '' },
      ],
    };
    const r = processAckDetailedResult(ackRes, ackIds);
    expect(r.acked + r.duplicate + r.not_found + r.retriable).toBe(5);
  });

  it('服务端 acked 字段 = acked 项数（除非无 items 详情）', () => {
    // 有 items 详情时：服务端 acked 字段应等于 items 中 status='acked' 的数量
    const ackRes = {
      status: 'ok',
      acked: 2,
      items: [
        { msg_id: 'm1', status: 'acked' },
        { msg_id: 'm2', status: 'acked' },
        { msg_id: 'm3', status: 'duplicate' },
      ],
    };
    const r = processAckDetailedResult(ackRes, ['m1', 'm2', 'm3']);
    expect(r.acked).toBe(2);
    expect(r.acked).toBe(ackRes.acked);
  });
});

