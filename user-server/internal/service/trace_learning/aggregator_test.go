package trace_learning

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractContent(t *testing.T) {
	assert.Equal(t, "你好", extractContent(`{"content":"你好","x":1}`))
	assert.Equal(t, "", extractContent(""))
	assert.Equal(t, "", extractContent("not json"))
	assert.Equal(t, "", extractContent(`{"other":1}`))
}

func TestDedupeStrings(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, dedupeStrings([]string{"a", "b", "a", "c", "b"}))
	assert.Nil(t, dedupeStrings(nil))
}

