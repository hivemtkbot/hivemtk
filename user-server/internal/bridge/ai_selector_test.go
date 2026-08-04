package bridge

import "testing"

// TestSanitizeExtractor 验证抽取器源码静态安全校验：
//   - 拒绝危险写法（fetch / localStorage / eval ...）
//   - 接受普通函数与箭头函数（与服务端前端 selector-ai.js 口径一致，避免 LLM
//     返回箭头函数时被服务端拒绝而前端放行的不一致）
func TestSanitizeExtractor(t *testing.T) {
	cases := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{name: "普通函数合法", code: `function(doc, win){ var els=doc.querySelectorAll('.b'); var m=[]; els.forEach(function(e){ m.push({text:e.textContent, self:false}); }); return {messages:m}; }`, wantErr: false},
		{name: "箭头函数合法", code: `(doc, win) => { const els = doc.querySelectorAll('.b'); return { messages: Array.from(els).map(e => ({ text: e.textContent, self: false })) }; }`, wantErr: false},
		{name: "箭头函数隐式返回合法", code: `(doc, win) => ({ messages: Array.from(doc.querySelectorAll('.b')).map(e => ({ text: e.textContent, self: false })) })`, wantErr: false},
		{name: "非函数体拒绝", code: `var x = 1;`, wantErr: true},
		{name: "fetch 危险写法拒绝", code: `function(d,w){ fetch('http://evil'); return {messages:[]}; }`, wantErr: true},
		{name: "localStorage 危险写法拒绝", code: `function(d,w){ localStorage.getItem('k'); return {messages:[]}; }`, wantErr: true},
		{name: "eval 危险写法拒绝", code: `function(d,w){ eval('1+1'); return {messages:[]}; }`, wantErr: true},
		{name: "箭头函数含 fetch 也拒绝", code: `(d, w) => { fetch('x'); return { messages: [] }; }`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := sanitizeExtractor(tc.code)
			if tc.wantErr && err == nil {
				t.Fatalf("期望拒绝，实际放行")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("期望放行，实际拒绝: %v", err)
			}
		})
	}
}
