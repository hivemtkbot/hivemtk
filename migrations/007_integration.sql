-- 第三方对接模块迁移脚本
-- 包含 CRM 对接（销售易、纷享销客）和电商对接（淘宝、京东）
-- 版本: 1.1.0
-- 适用于: PostgreSQL 15+ (项目唯一数据库)

CREATE TABLE IF NOT EXISTS integration_accounts (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,  -- crm_xiaoshouyi, crm_fenxiangxiao, ecommerce_taobao, ecommerce_jd
    account_name VARCHAR(100),
    api_key VARCHAR(200),
    api_secret VARCHAR(200),
    refresh_token TEXT,
    access_token TEXT,
    token_expires TIMESTAMP,
    webhook_url VARCHAR(500),
    config TEXT,  -- JSON 配置
    status INTEGER DEFAULT 1,  -- 1-启用 0-禁用
    last_sync_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_integration_accounts_platform ON integration_accounts(platform);

CREATE OR REPLACE FUNCTION integration_accounts_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_integration_accounts_updated_at ON integration_accounts;
CREATE TRIGGER trg_integration_accounts_updated_at
    BEFORE UPDATE ON integration_accounts
    FOR EACH ROW EXECUTE FUNCTION integration_accounts_set_updated_at();

CREATE TABLE IF NOT EXISTS sync_logs (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,
    sync_type VARCHAR(50),  -- customer, order, product 等
    status INTEGER DEFAULT 0,  -- 0-进行中 1-成功 2-失败
    record_count INTEGER DEFAULT 0,
    error_message TEXT,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sync_logs_platform ON sync_logs(platform);

CREATE TABLE IF NOT EXISTS external_customers (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,
    external_id VARCHAR(100) NOT NULL,  -- 外部系统客户 ID
    name VARCHAR(100),
    phone VARCHAR(50),
    email VARCHAR(100),
    company VARCHAR(200),
    position VARCHAR(100),
    industry VARCHAR(100),
    level VARCHAR(50),  -- 客户级别
    source VARCHAR(100),
    owner_id VARCHAR(100),  -- 负责人 ID
    owner_name VARCHAR(100),
    status VARCHAR(50),  -- 潜在客户、意向客户、成交客户等
    tags TEXT,  -- JSON 数组
    last_contact_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_external_customers_platform ON external_customers(platform);
CREATE INDEX IF NOT EXISTS idx_external_customers_external_id ON external_customers(external_id);
CREATE INDEX IF NOT EXISTS idx_external_customers_phone ON external_customers(phone);

CREATE OR REPLACE FUNCTION external_customers_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_external_customers_updated_at ON external_customers;
CREATE TRIGGER trg_external_customers_updated_at
    BEFORE UPDATE ON external_customers
    FOR EACH ROW EXECUTE FUNCTION external_customers_set_updated_at();

CREATE TABLE IF NOT EXISTS external_orders (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,
    order_id VARCHAR(100) NOT NULL,  -- 外部订单号
    order_no VARCHAR(100),  -- 内部订单号
    user_id VARCHAR(100),
    user_name VARCHAR(100),
    user_phone VARCHAR(50),
    total_amount DECIMAL(10,2),
    pay_amount DECIMAL(10,2),
    discount_amount DECIMAL(10,2),
    status VARCHAR(50),  -- 待付款、已付款、发货中、已完成、已取消
    pay_time TIMESTAMP,
    ship_time TIMESTAMP,
    complete_time TIMESTAMP,
    items TEXT,  -- JSON 数组
    shipping_addr TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_external_orders_platform ON external_orders(platform);
CREATE UNIQUE INDEX IF NOT EXISTS idx_external_orders_order_id ON external_orders(order_id);
CREATE INDEX IF NOT EXISTS idx_external_orders_user_id ON external_orders(user_id);

-- updated_at 自动更新触发器 (external_orders)
CREATE OR REPLACE FUNCTION external_orders_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_external_orders_updated_at ON external_orders;
CREATE TRIGGER trg_external_orders_updated_at
    BEFORE UPDATE ON external_orders
    FOR EACH ROW EXECUTE FUNCTION external_orders_set_updated_at();

-- 外部商品表（电商对接）
CREATE TABLE IF NOT EXISTS external_products (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,
    product_id VARCHAR(100) NOT NULL,
    name VARCHAR(200),
    category_id VARCHAR(100),
    category_name VARCHAR(100),
    price DECIMAL(10,2),
    original_price DECIMAL(10,2),
    stock INTEGER DEFAULT 0,
    sales INTEGER DEFAULT 0,  -- 销量
    images TEXT,  -- JSON 数组
    status INTEGER DEFAULT 1,  -- 1-上架 0-下架
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_external_products_platform ON external_products(platform);
CREATE INDEX IF NOT EXISTS idx_external_products_product_id ON external_products(product_id);

-- updated_at 自动更新触发器 (external_products)
CREATE OR REPLACE FUNCTION external_products_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_external_products_updated_at ON external_products;
CREATE TRIGGER trg_external_products_updated_at
    BEFORE UPDATE ON external_products
    FOR EACH ROW EXECUTE FUNCTION external_products_set_updated_at();

-- Webhook 事件表
CREATE TABLE IF NOT EXISTS webhook_events (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,
    event_id VARCHAR(100) UNIQUE NOT NULL,
    event_type VARCHAR(50),  -- customer.created, order.paid 等
    raw_data TEXT,
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_webhook_events_platform ON webhook_events(platform);
CREATE INDEX IF NOT EXISTS idx_webhook_events_processed ON webhook_events(processed);
