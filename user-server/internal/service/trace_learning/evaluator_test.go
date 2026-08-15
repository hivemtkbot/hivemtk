package trace_learning

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEvalResult(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
		wantBad bool
		want    int
	}{
		{
			name:    "裸JSON",
			raw:     `{"score":30,"dimensions":{"relevance":10,"accuracy":20,"usefulness":10,"safety":80},"reason":"答非所问","bad":true}`,
			wantErr: false, wantBad: true, want: 30,
		},
		{
			name:    "fenced代码块",
			raw:     "```json\n{\"score\":90,\"dimensions\":{\"relevance\":90,\"accuracy\":95,\"usefulness\":88,\"safety\":100},\"reason\":\"ok\",\"bad\":false}\n```",
			wantErr: false, wantBad: false, want: 90,
		},
		{name: "空返回", raw: "", wantErr: true},
		{name: "非JSON", raw: "not json at all", wantErr: true},
		{name: "超界高位截断到100", raw: `{"score":250,"dimensions":{},"bad":false}`, wantErr: false, want: 100},
		{name: "超界低位截断到0", raw: `{"score":-5,"dimensions":{},"bad":false}`, wantErr: false, want: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := parseEvalResult(c.raw)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, r.Score)
			assert.Equal(t, c.wantBad, r.Bad)
			assert.NotNil(t, r.Dimensions, "维度应至少初始化为空 map")
		})
	}
}

