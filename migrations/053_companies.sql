-- USR-CM-02: 公司维度
CREATE TABLE IF NOT EXISTS companies (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    domain VARCHAR(200),
    industry VARCHAR(50),
    size VARCHAR(20), -- 'startup' | 'smb' | 'enterprise'
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_companies_domain ON companies(domain) WHERE deleted_at IS NULL;

-- 客户关联公司
ALTER TABLE customers ADD COLUMN IF NOT EXISTS company_id BIGINT REFERENCES companies(id);
CREATE INDEX IF NOT EXISTS idx_customers_company ON customers(company_id) WHERE deleted_at IS NULL;

COMMENT ON TABLE companies IS '公司维度（USR-CM-02）：B2B 业务必备';
