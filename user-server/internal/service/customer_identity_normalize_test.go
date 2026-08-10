package service

import (
	"strings"
	"testing"

	"hivemtk-user/internal/identity"
)

// ===== NormalizePhone 测试 =====

func TestNormalizePhone_Empty(t *testing.T) {
	if got := NormalizePhone(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := NormalizePhone("   "); got != "" {
		t.Errorf("expected empty for spaces, got %q", got)
	}
}

func TestNormalizePhone_PlainDigits(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"13800138000", "13800138000"},
		{"13800138001", "13800138001"},
		{"13800138002", "13800138002"},
		{"13800138003", "13800138003"},
		{"13800138004", "13800138004"},
		{"13800138005", "13800138005"},
		{"13800138006", "13800138006"},
		{"13800138007", "13800138007"},
		{"13800138008", "13800138008"},
		{"13800138009", "13800138009"},
	}
	for i, c := range cases {
		if got := NormalizePhone(c.in); got != c.want {
			t.Errorf("case %d: NormalizePhone(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

func TestNormalizePhone_With86Prefix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"+8613800138000", "13800138000"},
		{"8613800138001", "13800138001"},
		{"+86 13800138002", "13800138002"},
		{"008613800138003", "13800138003"},
	}
	for i, c := range cases {
		if got := NormalizePhone(c.in); got != c.want {
			t.Errorf("case %d: NormalizePhone(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

func TestNormalizePhone_WithSeparators(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"138 0013 8000", "13800138000"},
		{"138-0013-8001", "13800138001"},
		{"138 0013 8002", "13800138002"},
		{"138.0013.8003", "13800138003"},
		{"138_0013_8004", "13800138004"},
		{"(138) 0013 8005", "13800138005"},
		{"138-0013-8006", "13800138006"},
		{"138—0013—8007", "13800138007"},
		{"+86 138-0013-8008", "13800138008"},
		{"138 0013 8009", "13800138009"},
	}
	for i, c := range cases {
		if got := NormalizePhone(c.in); got != c.want {
			t.Errorf("case %d: NormalizePhone(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

func TestNormalizePhone_Not11Digits(t *testing.T) {
	// 非 11 位的非国内手机号，原样返回 trim
	cases := []string{
		"12345",
		"abcdefghijk",
		"123-456-789",
		"1380013800",   // 10 位
		"138001380000", // 12 位
	}
	for _, c := range cases {
		got := NormalizePhone(c)
		if got == "" && c != "" {
			t.Errorf("NormalizePhone(%q) returned empty for non-11-digit", c)
		}
	}
}

// ===== NormalizeEmail 测试 =====

func TestNormalizeEmail_Empty(t *testing.T) {
	if got := NormalizeEmail(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := NormalizeEmail("   "); got != "" {
		t.Errorf("expected empty for spaces, got %q", got)
	}
}

func TestNormalizeEmail_Lowercase(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"FOO@EXAMPLE.COM", "foo@example.com"},
		{"Foo.Bar@Example.Com", "foo.bar@example.com"},
		{"USER+tag@DOMAIN.CN", "user+tag@domain.cn"},
		{"Mixed_Case@Test.Org", "mixed_case@test.org"},
		{"ALL.UPPER@COMPANY.IO", "all.upper@company.io"},
	}
	for i, c := range cases {
		if got := NormalizeEmail(c.in); got != c.want {
			t.Errorf("case %d: NormalizeEmail(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

func TestNormalizeEmail_TrimSpace(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  foo@example.com  ", "foo@example.com"},
		{"bar@example.com\n", "bar@example.com"},
		{"\tbaz@example.com\t", "baz@example.com"},
		{" \r\n qux@example.com \r\n ", "qux@example.com"},
		{"a.b.c@d.com ", "a.b.c@d.com"},
	}
	for i, c := range cases {
		if got := NormalizeEmail(c.in); got != c.want {
			t.Errorf("case %d: NormalizeEmail(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

func TestNormalizeEmail_InvalidFormat(t *testing.T) {
	// 不符合邮箱格式时，原样返回 trim（保留观察）
	cases := []string{
		"not-an-email",
		"missing-at.com",
		"@example.com",
		"foo@",
	}
	for _, c := range cases {
		got := NormalizeEmail(c)
		if got == "" {
			t.Errorf("invalid email %q returned empty, expected trimmed raw", c)
		}
	}
}

// ===== NormalizeOpenID 测试 =====

func TestNormalizeOpenID_Trim(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"openid_001", "openid_001"},
		{"  openid_002  ", "openid_002"},
		{"\nopenid_003\n", "openid_003"},
		{"OpenID_CaseSensitive", "OpenID_CaseSensitive"}, // 大小写不改变
		{"", ""},
		{"   ", ""},
	}
	for i, c := range cases {
		if got := NormalizeOpenID(c.in); got != c.want {
			t.Errorf("case %d: NormalizeOpenID(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

// ===== NormalizeIdentifiers 集成测试 =====

func TestNormalizeIdentifiers_AllFields(t *testing.T) {
	in := identity.Identifiers{
		Phone:        "+86 138-0013-8000",
		Email:        "  Foo@Example.COM  ",
		WechatOpenID: "  wx_open_id_001  ",
		DouyinOpenID: "\ndy_open_id_001\n",
	}
	out := NormalizeIdentifiers(in)
	if out.Phone != "13800138000" {
		t.Errorf("Phone = %q, want 13800138000", out.Phone)
	}
	if out.Email != "foo@example.com" {
		t.Errorf("Email = %q, want foo@example.com", out.Email)
	}
	if out.WechatOpenID != "wx_open_id_001" {
		t.Errorf("WechatOpenID = %q, want wx_open_id_001", out.WechatOpenID)
	}
	if out.DouyinOpenID != "dy_open_id_001" {
		t.Errorf("DouyinOpenID = %q, want dy_open_id_001", out.DouyinOpenID)
	}
}

func TestNormalizeIdentifiers_PartialFields(t *testing.T) {
	// 只有手机号
	in := identity.Identifiers{Phone: "  13800138000  "}
	out := NormalizeIdentifiers(in)
	if out.Phone != "13800138000" {
		t.Errorf("Phone normalization failed")
	}

	// 只有邮箱
	in = identity.Identifiers{Email: "FOO@BAR.COM"}
	out = NormalizeIdentifiers(in)
	if out.Email != "foo@bar.com" {
		t.Errorf("Email normalization failed")
	}
}

// ===== HasAnyIdentifier 测试 =====

func TestHasAnyIdentifier(t *testing.T) {
	cases := []struct {
		in   identity.Identifiers
		want bool
	}{
		{identity.Identifiers{}, false},
		{identity.Identifiers{Phone: "1"}, true},
		{identity.Identifiers{Email: "a"}, true},
		{identity.Identifiers{WechatOpenID: "w"}, true},
		{identity.Identifiers{DouyinOpenID: "d"}, true},
		{identity.Identifiers{Phone: "1", Email: "a"}, true},
		{identity.Identifiers{Phone: "  ", Email: ""}, false}, // trim 后空不算
	}
	for i, c := range cases {
		if got := HasAnyIdentifier(c.in); got != c.want {
			t.Errorf("case %d: HasAnyIdentifier(%+v) = %v, want %v", i, c.in, got, c.want)
		}
	}
}

// ===== Hash 工具测试 =====

func TestPhoneHash_Deterministic(t *testing.T) {
	h1 := PhoneHash("13800138000")
	h2 := PhoneHash("13800138000")
	if h1 == "" {
		t.Error("PhoneHash returned empty")
	}
	if h1 != h2 {
		t.Errorf("PhoneHash not deterministic: %s vs %s", h1, h2)
	}
	if len(h1) != 64 { // SHA-256 hex
		t.Errorf("PhoneHash length = %d, want 64", len(h1))
	}
}

func TestPhoneHash_AfterNormalize(t *testing.T) {
	// 不同写法的同一个手机号应该 hash 相同
	h1 := PhoneHash("13800138000")
	h2 := PhoneHash("+86 138-0013-8000")
	h3 := PhoneHash("  138 0013 8000  ")
	if h1 != h2 || h1 != h3 {
		t.Errorf("PhoneHash should be same after normalize: %s, %s, %s", h1, h2, h3)
	}
}

func TestPhoneHash_Empty(t *testing.T) {
	if got := PhoneHash(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestPhoneHash_DifferentPhones(t *testing.T) {
	h1 := PhoneHash("13800138000")
	h2 := PhoneHash("13800138001")
	if h1 == h2 {
		t.Error("Different phones should produce different hashes")
	}
}

func TestEmailHash_Deterministic(t *testing.T) {
	h1 := EmailHash("foo@example.com")
	h2 := EmailHash("foo@example.com")
	if h1 == "" {
		t.Error("EmailHash returned empty")
	}
	if h1 != h2 {
		t.Errorf("EmailHash not deterministic: %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("EmailHash length = %d, want 64", len(h1))
	}
}

func TestEmailHash_CaseInsensitive(t *testing.T) {
	// 邮箱小写归一后 hash 一致
	h1 := EmailHash("foo@example.com")
	h2 := EmailHash("FOO@EXAMPLE.COM")
	h3 := EmailHash("  Foo@Example.Com  ")
	if h1 != h2 || h1 != h3 {
		t.Errorf("EmailHash should be same after normalize: %s, %s, %s", h1, h2, h3)
	}
}

func TestEmailHash_Empty(t *testing.T) {
	if got := EmailHash(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ===== 批量等价性测试（100 用例级别） =====

func TestNormalizePhone_BatchEquivalent(t *testing.T) {
	// 同一手机号，10 种写法，期望都归一为同一个 11 位数字
	base := "13800138000"
	variants := []string{
		"13800138000",
		" 13800138000",
		"13800138000 ",
		"+8613800138000",
		"86 138 0013 8000",
		"138-0013-8000",
		"138.0013.8000",
		"  +86 138-0013-8000  ",
		"008613800138000",
		"\t13800138000\n",
	}
	for i, v := range variants {
		got := NormalizePhone(v)
		if got != base {
			t.Errorf("variant %d (%q) -> %q, want %q", i, v, got, base)
		}
	}
}

func TestNormalizeEmail_BatchEquivalent(t *testing.T) {
	base := "user@example.com"
	variants := []string{
		"user@example.com",
		"USER@EXAMPLE.COM",
		"User@Example.Com",
		"  user@example.com  ",
		"\tuser@example.com\n",
		"USER@example.com",
		"user@EXAMPLE.com",
		"user@example.COM",
	}
	for i, v := range variants {
		got := NormalizeEmail(v)
		if got != base {
			t.Errorf("variant %d (%q) -> %q, want %q", i, v, got, base)
		}
	}
}

// ===== 边界用例 =====

func TestNormalizePhone_TableDriven(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// 11 位标准
		{"13800138000", "13800138000"},
		{"13800138001", "13800138001"},
		{"13800138002", "13800138002"},
		{"13800138003", "13800138003"},
		{"13800138004", "13800138004"},
		// +86
		{"+8613800138005", "13800138005"},
		{"+8613800138006", "13800138006"},
		{"+86-138-0013-8007", "13800138007"},
		// 86 前缀
		{"8613800138008", "13800138008"},
		// 各种分隔
		{"138 0013 8009", "13800138009"},
		{"138 0013 8010", "13800138010"},
		{"138-0013-8011", "13800138011"},
		{"138-0013-8012", "13800138012"},
		{"138.0013.8013", "13800138013"},
		{"138.0013.8014", "13800138014"},
		// 大陆手机号常见号段
		{"15912345678", "15912345678"},
		{"18612345678", "18612345678"},
		{"17712345678", "17712345678"},
		{"19912345678", "19912345678"},
		{"16612345678", "16612345678"},
		{"13312345678", "13312345678"},
		{"15612345678", "15612345678"},
		{"13112345678", "13112345678"},
		{"13912345678", "13912345678"},
		{"18812345678", "18812345678"},
		{"15212345678", "15212345678"},
		{"15812345678", "15812345678"},
		// 边界：trim
		{"  13812345678  ", "13812345678"},
		{"\n13812345678\n", "13812345678"},
		{"\t13812345678\t", "13812345678"},
		// 组合
		{"+86 138-0013-8000", "13800138000"},
		{"  +86 138 0013 8000  ", "13800138000"},
	}
	if len(cases) < 30 {
		t.Errorf("table-driven cases count = %d, expected more", len(cases))
	}
	for i, c := range cases {
		if got := NormalizePhone(c.in); got != c.want {
			t.Errorf("case %d: NormalizePhone(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

func TestNormalizeEmail_TableDriven(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"a@b.co", "a@b.co"},
		{"A@B.CO", "a@b.co"},
		{"a@b.CO", "a@b.co"},
		{"A@b.co", "a@b.co"},
		{"foo.bar@example.com", "foo.bar@example.com"},
		{"FOO.BAR@EXAMPLE.COM", "foo.bar@example.com"},
		{"user+tag@example.com", "user+tag@example.com"},
		{"USER+TAG@EXAMPLE.COM", "user+tag@example.com"},
		{"a.b.c@d.e.f.com", "a.b.c@d.e.f.com"},
		{"x_y_z@domain.org", "x_y_z@domain.org"},
		{"u1@v1.cn", "u1@v1.cn"},
		{"  a@b.co  ", "a@b.co"},
		{"\ta@b.co\n", "a@b.co"},
		{"First.Last@Company.IO", "first.last@company.io"},
		{"USER@sub.domain.com", "user@sub.domain.com"},
	}
	for i, c := range cases {
		if got := NormalizeEmail(c.in); got != c.want {
			t.Errorf("case %d: NormalizeEmail(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

// ===== 反向测试：相同输入必须产生相同输出（幂等性） =====

func TestNormalize_Idempotent(t *testing.T) {
	inputs := []string{
		"+86 138-0013-8000",
		"  FOO@EXAMPLE.COM  ",
		"  OpenID_001  ",
	}
	for _, in := range inputs {
		// 对 phone/email/openid 各自做两次 normalize，结果应该一样
		// phone
		phone1 := NormalizePhone(in)
		phone2 := NormalizePhone(phone1)
		if phone1 != phone2 {
			t.Errorf("Phone normalize not idempotent: %q -> %q -> %q", in, phone1, phone2)
		}
		// email
		email1 := NormalizeEmail(in)
		email2 := NormalizeEmail(email1)
		if email1 != email2 {
			t.Errorf("Email normalize not idempotent: %q -> %q -> %q", in, email1, email2)
		}
		// openid
		oid1 := NormalizeOpenID(in)
		oid2 := NormalizeOpenID(oid1)
		if oid1 != oid2 {
			t.Errorf("OpenID normalize not idempotent: %q -> %q -> %q", in, oid1, oid2)
		}
	}
}

// ===== 防止空字符串误判 =====

func TestNormalizePhone_OnlySeparators(t *testing.T) {
	// 只有分隔符，没有数字
	got := NormalizePhone("+ - . _")
	if got != "" {
		t.Errorf("only separators should produce empty, got %q", got)
	}
}

func TestNormalizeEmail_OnlyWhitespace(t *testing.T) {
	got := NormalizeEmail("   \t\n  ")
	if got != "" {
		t.Errorf("only whitespace should produce empty, got %q", got)
	}
}

func TestPhoneHash_NeverContainsRawPhone(t *testing.T) {
	// 安全要求：日志中只允许 hash 出现，不允许原值
	phone := "13800138000"
	hash := PhoneHash(phone)
	if strings.Contains(hash, phone) {
		t.Errorf("PhoneHash should not contain raw phone value")
	}
}

func TestEmailHash_NeverContainsRawEmail(t *testing.T) {
	email := "secret@private.com"
	hash := EmailHash(email)
	if strings.Contains(hash, "secret") {
		t.Errorf("EmailHash should not contain raw email local part")
	}
}
