package llm

import (
	"hivemtk-user/internal/secrets"

	"hivemtk-user/internal/pkg/utils/logger"
)

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

func decryptAPIKeyForUse(stored string) string {
	if stored == "" || !secrets.IsCiphertextFormat(stored) {
		return stored
	}
	plain, err := secrets.DecryptString(stored)
	if err != nil {
		logger.Errorf("[LLM] api key decrypt failed (master key rotation/多实例 key 不一致?), provider 将以空 key 显式失败: %v", err)
		return ""
	}
	return plain
}
