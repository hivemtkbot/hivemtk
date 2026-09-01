package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/pkg/crypto"

	"gorm.io/gorm"
)

// EmailSmtpPasswordEncryptMigration R50 配套：email_smtp 存量明文密码一次性加密
//
// 背景：R50 UI 实测发现 CreateEmailSmtp 的 fail-open 分支在 FIELD_ENCRYPTION_KEY
// 未配置时静默跳过加密，导致存量 email_smtp.password 以明文落库（25+ 行）。
// R50 已将 Create/Update 收口为 fail-closed（加密失败拒绝写入）。
//
// 本迁移把存量明文密码一次性读-加密-写回：
//   - 判定规则沿用读取侧双读契约：以 base64 解码 + 前缀非 "{" 的历史约定为准，
//     更稳妥的判定 = 尝试 Decrypt 成功即为密文，失败视为明文
//   - 密钥未配置 / 无效时整迁移失败（fail-closed，与 R50 语义一致）
//   - 幂等安全：明文行才写回，密文行跳过
//
// ⚠️ 运维前置：必须先注入 FIELD_ENCRYPTION_KEY（≥32字节）再执行，否则 Up 返回错误。
type EmailSmtpPasswordEncryptMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*EmailSmtpPasswordEncryptMigration)(nil)

func NewEmailSmtpPasswordEncryptMigration(db *gorm.DB) *EmailSmtpPasswordEncryptMigration {
	return &EmailSmtpPasswordEncryptMigration{db: db}
}

func (m *EmailSmtpPasswordEncryptMigration) Version() string { return "v3.28.0" }

func (m *EmailSmtpPasswordEncryptMigration) Name() string {
	return "email_smtp 存量明文密码一次性 AES-GCM 加密"
}

func (m *EmailSmtpPasswordEncryptMigration) Description() string {
	return "R50 fail-closed 配套: 把 FIELD_ENCRYPTION_KEY 缺失期间明文落库的 SMTP 密码批量加密"
}

func (m *EmailSmtpPasswordEncryptMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 密钥可用性预检（fail-closed：无密钥时迁移失败并提示，绝不留半加密状态）
	if err := crypto.Init(); err != nil {
		return fmt.Errorf("FIELD_ENCRYPTION_KEY 不可用, 拒绝执行明文密码加密迁移: %w", err)
	}

	type row struct {
		ID       string
		Password string
	}
	var rows []row
	if err := m.db.WithContext(ctx).
		Table("email_smtp").
		Select("id, password").
		Where("password IS NOT NULL AND password != ''").
		Find(&rows).Error; err != nil {
		return fmt.Errorf("查询 email_smtp 失败: %w", err)
	}

	for _, r := range rows {
		// 已是密文（Decrypt 成功且解出非空原文）→ 跳过，保证幂等；
		// 明文行 base64 解码几乎必然失败或解不出合法 nonce 结构 → 走加密分支
		if plain, derr := crypto.Decrypt(r.Password); derr == nil && plain != "" {
			continue
		}
		enc, err := crypto.Encrypt(r.Password)
		if err != nil {
			return fmt.Errorf("加密 email_smtp %s 密码失败: %w", r.ID, err)
		}
		if enc == r.Password {
			return fmt.Errorf("email_smtp %s 加密输出与输入一致, 异常终止", r.ID)
		}
		if err := m.db.WithContext(ctx).
			Table("email_smtp").
			Where("id = ?", r.ID).
			Update("password", enc).Error; err != nil {
			return fmt.Errorf("回写 email_smtp %s 密文失败: %w", r.ID, err)
		}
	}
	return nil
}

func (m *EmailSmtpPasswordEncryptMigration) Down(ctx context.Context) error {
	// 单向迁移：密码解密回明文属于安全倒退，不支持 Down
	return fmt.Errorf("不支持回滚: 密码解密回明文是安全倒退")
}
