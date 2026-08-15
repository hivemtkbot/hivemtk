package ragretrieval

import (
	"testing"

	"hivemtk-user/internal/config"

	"github.com/stretchr/testify/assert"
)

// TestNewIndexManager_UnsupportedType 不支持的向量库类型返回错误
func TestNewIndexManager_UnsupportedType(t *testing.T) {
	_, err := NewIndexManager(config.VectorDatabaseConfig{Type: "milvus"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported vector database type")
}

// TestNewIndexManager_PGVector pgvector 类型回退到内存实现
func TestNewIndexManager_PGVector(t *testing.T) {
	mgr, err := NewIndexManager(config.VectorDatabaseConfig{Type: config.VectorDBTypePGVector})
	assert.NoError(t, err)
	assert.NotNil(t, mgr)
	im, ok := mgr.(*InMemoryIndexManager)
	assert.True(t, ok)
	assert.Equal(t, 512, im.dimension)
}

// TestNewIndexManagerWithDB_UnsupportedType 带 DB 工厂的不支持类型返回错误
func TestNewIndexManagerWithDB_UnsupportedType(t *testing.T) {
	_, err := NewIndexManagerWithDB(nil, config.VectorDatabaseConfig{Type: "pinecone"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported vector database type")
}

// TestNewIndexManagerWithDB_PGVector pgvector 类型回退到内存实现
func TestNewIndexManagerWithDB_PGVector(t *testing.T) {
	mgr, err := NewIndexManagerWithDB(nil, config.VectorDatabaseConfig{Type: config.VectorDBTypePGVector})
	assert.NoError(t, err)
	assert.NotNil(t, mgr)
	im, ok := mgr.(*InMemoryIndexManager)
	assert.True(t, ok, "expected *InMemoryIndexManager after PGVector dead-code removal")
	assert.Equal(t, 512, im.dimension)
}

