package llm

// LLM Provider API Key 加密存储 —— ChatbotX 负面教训直接应用（T6）。
//
// 背景：ChatbotX 的 AI provider BYOK key 以明文存 jsonb（其 AES-256-GCM 工具
// 只用于营销集成），是已知安全债。hivemtk 的 llm_providers.api_key 同样明文落库。
// 本文件给 provider 持久化层接上 internal/secrets 的 AES-256-GCM：
//   - 写库：EncryptString（enc:v1:{base64(nonce|ct)} 格式，与仓库既有约定一致）；
//   - 读库：IsCiphertextFormat 判别 → DecryptString；存量明文双轨直读；
//   - 启动：LoadProvidersFromDB 内对明文行做一次性读-加密-回写（幂等）。
// master key 未配置（secrets 未 Ready）时全链路退化为明文（与仓库降级哲学一致），
// 绝不产出不可恢复密文。
import (
	"hivemtk-user/internal/secrets"

	"hivemtk-user/internal/pkg/utils/logger"
)

// encryptAPIKeyForStorage API Key 入库前加密。
// secrets 未就绪 / 空 key / 已是密文格式 → 原样返回（幂等）。
func encryptAPIKeyForStorage(plain string) string {
	if plain == "" || secrets.IsCiphertextFormat(plain) || !secrets.Ready() {
		return plain
	}
	enc, err := secrets.EncryptString(plain)
	if err != nil {
		logger.Warnf("[LLM] api key encrypt failed (store plaintext as fallback): %v", err)
		return plain
	}
	return enc
}

// decryptAPIKeyForUse 读库后解密。
// 明文（存量/未启用加密）→ 原样返回；密文解密失败 → 原样返回并告警
// （master key 轮换丢失场景，保留原值便于运维排查）。
func decryptAPIKeyForUse(stored string) string {
	if stored == "" || !secrets.IsCiphertextFormat(stored) {
		return stored
	}
	plain, err := secrets.DecryptString(stored)
	if err != nil {
		logger.Errorf("[LLM] api key decrypt failed (key rotation lost?), using stored value: %v", err)
		return stored
	}
	return plain
}
