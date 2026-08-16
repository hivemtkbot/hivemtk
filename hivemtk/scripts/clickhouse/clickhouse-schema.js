/**
 * ClickHouse 加速实时看板（USR-AN-04）
 * 借鉴：https://clickhouse.com/docs
 *
 * 策略：
 * - 强一致数据（财务/客户/订单）→ PostgreSQL
 * - 弱一致事件流（埋点/消息/统计）→ ClickHouse 旁路
 * - 大屏查询默认走 CH，5s 延迟可接受
 *
 * 关键设计：
 * 1. 双写：业务写 PG + 异步推 CH（Kafka 或 outbox pattern）
 * 2. 表结构：CH 用 MergeTree 引擎，partition by toYYYYMM(event_at)
 * 3. 查询：物化视图预聚合常用 KPI
 */

const CH_SCHEMA_SQL = `
-- ClickHouse 表结构（USR-AN-04）

-- 1. 客户事件流
CREATE TABLE IF NOT EXISTS customer_events_ch (
  user_id String,
  event_type LowCardinality(String),
  properties String,
  occurred_at DateTime,
  created_at DateTime DEFAULT now()
) ENGINE = MergeTree
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (event_type, occurred_at, user_id)
TTL occurred_at + INTERVAL 90 DAY;

-- 2. 消息事件流
CREATE TABLE IF NOT EXISTS message_events_ch (
  session_id String,
  message_id String,
  channel LowCardinality(String),
  event_type LowCardinality(String), -- sent / delivered / read / failed
  occurred_at DateTime,
  latency_ms UInt32
) ENGINE = MergeTree
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (channel, event_type, occurred_at);

-- 3. 物化视图：每日会话数
CREATE MATERIALIZED VIEW IF NOT EXISTS daily_session_count_mv
ENGINE = SummingMergeTree
PARTITION BY toYYYYMM(day)
ORDER BY (day, channel)
POPULATE AS
SELECT
  toDate(occurred_at) AS day,
  channel,
  count() AS sessions
FROM message_events_ch
WHERE event_type = 'sent'
GROUP BY day, channel;

-- 4. 物化视图：5 分钟活跃用户
CREATE MATERIALIZED VIEW IF NOT EXISTS mau_5min_mv
ENGINE = AggregatingMergeTree
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (user_id, occurred_at)
AS
SELECT
  user_id,
  toStartOfFiveMinute(occurred_at) AS occurred_at,
  uniqState(user_id) AS cnt
FROM customer_events_ch
GROUP BY user_id, occurred_at;
`

// 双写客户端
class DualWriter {
  constructor(pgClient, chClient) {
    this.pg = pgClient
    this.ch = chClient
  }

  async insertCustomerEvent(event) {
    // 1. 强一致：写 PG
    await this.pg.insert('customer_events', event)
    // 2. 异步：推 CH（失败也不影响主流程）
    this.ch.insert('customer_events_ch', event).catch((e) => {
      console.warn('[DualWriter] CH 写入失败：', e)
    })
  }
}

module.exports = { CH_SCHEMA_SQL, DualWriter }
