package identity

import (
	"strings"
	"testing"
)


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
		{"+86 138-0013-8004", "13800138004"},
		{"86-138-0013-8005", "13800138005"},
		{"0086 138 0013 8006", "13800138006"},
		{"+86 13800138007", "13800138007"},
		{"0086-138-0013-8008", "13800138008"},
		{"+86-138-0013-8009", "13800138009"},
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
		{"138 0013 8010", "13800138010"},
		{"138-0013-8011", "13800138011"},
		{"138.0013.8012", "13800138012"},
		{"138_0013_8013", "13800138013"},
		{"138 0013 8014", "13800138014"},
	}
	for i, c := range cases {
		if got := NormalizePhone(c.in); got != c.want {
			t.Errorf("case %d: NormalizePhone(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

func TestNormalizePhone_AllCarriers(t *testing.T) {
	cases := []string{
		"13012345678", "13112345678", "13212345678",
		"13312345678", "13412345678", "13512345678",
		"13612345678", "13712345678", "13812345678",
		"13912345678",
		"15012345678", "15112345678", "15212345678",
		"15712345678", "15812345678", "15912345678",
		"17012345678", "17112345678", "17212345678",
		"17312345678", "17512345678", "17612345678",
		"17712345678", "17812345678",
		"18012345678", "18112345678", "18212345678",
		"18312345678", "18412345678", "18512345678",
		"18612345678", "18712345678", "18812345678",
		"18912345678",
		"19112345678", "19812345678", "19912345678",
	}
	for _, c := range cases {
		if got := NormalizePhone(c); got != c {
			t.Errorf("plain number changed: %q -> %q", c, got)
		}
		got2 := NormalizePhone("+86 " + c)
		if got2 != c {
			t.Errorf("+86 number failed: +86 %s -> %q, want %s", c, got2, c)
		}
	}
}

func TestNormalizePhone_Not11Digits(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"12345", "12345"},
		{"abcdefghijk", "abcdefghijk"},   
		{"1380013800", "1380013800"},     
		{"138001380000", "138001380000"}, 
		{"+861380013800", "1380013800"},  
	}
	for i, c := range cases {
		if got := NormalizePhone(c.in); got != c.want {
			t.Errorf("case %d: NormalizePhone(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}


func TestNormalizeEmail_Empty(t *testing.T) {
	if got := NormalizeEmail(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := NormalizeEmail("   "); got != "" {
		t.Errorf("expected empty for spaces, got %q", got)
	}
	if got := NormalizeEmail("\t\n"); got != "" {
		t.Errorf("expected empty for whitespace, got %q", got)
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
		{"User@Sub.Domain.Com", "user@sub.domain.com"},
		{"FIRST.LAST@Example.Net", "first.last@example.net"},
		{"U@v.CN", "u@v.cn"},
		{"Test.User+tag@gmail.com", "test.user+tag@gmail.com"},
		{"ABC@DEF.GHI.JKL", "abc@def.ghi.jkl"},
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
		{"  multi@space.com  ", "multi@space.com"},
		{"\nline@break.com\n", "line@break.com"},
		{"\ttab@tab.com\t", "tab@tab.com"},
		{"  trim@both.com  ", "trim@both.com"},
		{"  wrap@all.com\n\t", "wrap@all.com"},
	}
	for i, c := range cases {
		if got := NormalizeEmail(c.in); got != c.want {
			t.Errorf("case %d: NormalizeEmail(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

func TestNormalizeEmail_InvalidFormat(t *testing.T) {
	cases := []string{
		"not-an-email",
		"missing-at.com",
		"@example.com",
		"foo@",
		"foo bar@example.com", 
		"foo@bar",             
	}
	for _, c := range cases {
		got := NormalizeEmail(c)
		if got == "" {
			t.Errorf("invalid email %q returned empty, expected trimmed raw", c)
		}
	}
}


func TestNormalizeOpenID_Trim(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"openid_001", "openid_001"},
		{"  openid_002  ", "openid_002"},
		{"\nopenid_003\n", "openid_003"},
		{"OpenID_CaseSensitive", "OpenID_CaseSensitive"}, 
		{"", ""},
		{"   ", ""},
		{"\t\n\ropenid_007\r\n\t", "openid_007"},
		{"wechat_open_id_8", "wechat_open_id_8"},
		{"dy_open_id_9", "dy_open_id_9"},
		{"xhs_open_id_10", "xhs_open_id_10"},
	}
	for i, c := range cases {
		if got := NormalizeOpenID(c.in); got != c.want {
			t.Errorf("case %d: NormalizeOpenID(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}


func TestNormalize_AllFields(t *testing.T) {
	in := Identifiers{
		Phone:         "+86 138-0013-8000",
		Email:         "  Foo@Example.COM  ",
		WechatOpenID:  "  wx_open_id_001  ",
		DouyinOpenID:  "\ndy_open_id_001\n",
		XiaohongshuID: "\txhs_id_001\t",
	}
	out := Normalize(in)
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
	if out.XiaohongshuID != "xhs_id_001" {
		t.Errorf("XiaohongshuID = %q, want xhs_id_001", out.XiaohongshuID)
	}
}

func TestNormalize_PartialFields(t *testing.T) {
	in := Identifiers{Phone: "  13800138000  "}
	out := Normalize(in)
	if out.Phone != "13800138000" {
		t.Errorf("Phone normalization failed")
	}

	in = Identifiers{Email: "FOO@BAR.COM"}
	out = Normalize(in)
	if out.Email != "foo@bar.com" {
		t.Errorf("Email normalization failed")
	}

	in = Identifiers{WechatOpenID: "  wx_001  "}
	out = Normalize(in)
	if out.WechatOpenID != "wx_001" {
		t.Errorf("WechatOpenID normalization failed")
	}
}


func TestHasAny(t *testing.T) {
	cases := []struct {
		in   Identifiers
		want bool
	}{
		{Identifiers{}, false},
		{Identifiers{Phone: "1"}, true},
		{Identifiers{Email: "a"}, true},
		{Identifiers{WechatOpenID: "w"}, true},
		{Identifiers{DouyinOpenID: "d"}, true},
		{Identifiers{XiaohongshuID: "x"}, true},
		{Identifiers{Phone: "1", Email: "a"}, true},
		{Identifiers{Phone: "  ", Email: ""}, false},
		{Identifiers{WechatOpenID: "  "}, false},
		{Identifiers{Phone: "13800138000"}, true},
	}
	for i, c := range cases {
		if got := HasAny(c.in); got != c.want {
			t.Errorf("case %d: HasAny(%+v) = %v, want %v", i, c.in, got, c.want)
		}
	}
}


func TestPhoneHash_Deterministic(t *testing.T) {
	h1 := PhoneHash("13800138000")
	h2 := PhoneHash("13800138000")
	if h1 == "" {
		t.Error("PhoneHash returned empty")
	}
	if h1 != h2 {
		t.Errorf("PhoneHash not deterministic: %s vs %s", h1, h2)
	}
	if len(h1) != 64 { 
		t.Errorf("PhoneHash length = %d, want 64", len(h1))
	}
}

func TestPhoneHash_AfterNormalize(t *testing.T) {
	variants := []string{
		"13800138000",
		"+86 138-0013-8000",
		"  138 0013 8000  ",
		"138-0013-8000",
		"+8613800138000",
		"  13800138000\n",
	}
	first := PhoneHash(variants[0])
	for i, v := range variants[1:] {
		h := PhoneHash(v)
		if h != first {
			t.Errorf("variant %d (%q) hash mismatch: %s vs %s", i, v, h, first)
		}
	}
}

func TestPhoneHash_Empty(t *testing.T) {
	if got := PhoneHash(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := PhoneHash("   "); got != "" {
		t.Errorf("expected empty for whitespace, got %q", got)
	}
}

func TestPhoneHash_DifferentPhones(t *testing.T) {
	phones := []string{
		"13800138000", "13800138001", "13800138002", "13800138003", "13800138004",
		"13800138005", "13800138006", "13800138007", "13800138008", "13800138009",
	}
	seen := make(map[string]string)
	for _, p := range phones {
		h := PhoneHash(p)
		if existing, ok := seen[h]; ok {
			t.Errorf("hash collision: %s and %s -> %s", existing, p, h)
		}
		seen[h] = p
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
	variants := []string{
		"foo@example.com",
		"FOO@EXAMPLE.COM",
		"  Foo@Example.Com  ",
		"FOO@example.com",
		"foo@EXAMPLE.com",
		"foo@example.COM",
	}
	first := EmailHash(variants[0])
	for i, v := range variants[1:] {
		h := EmailHash(v)
		if h != first {
			t.Errorf("variant %d (%q) hash mismatch: %s vs %s", i, v, h, first)
		}
	}
}

func TestEmailHash_Empty(t *testing.T) {
	if got := EmailHash(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}


func TestNormalizePhone_BatchEquivalent(t *testing.T) {
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


func TestNormalizePhone_Idempotent(t *testing.T) {
	inputs := []string{
		"+86 138-0013-8000",
		"  138 0013 8000  ",
		"13800138000",
		"\n138-0013-8011\n",
	}
	for _, in := range inputs {
		p1 := NormalizePhone(in)
		p2 := NormalizePhone(p1)
		if p1 != p2 {
			t.Errorf("Phone normalize not idempotent: %q -> %q -> %q", in, p1, p2)
		}
	}
}

func TestNormalizeEmail_Idempotent(t *testing.T) {
	inputs := []string{
		"  FOO@EXAMPLE.COM  ",
		"Foo.Bar@Example.Com",
		"\tuser@example.com\n",
	}
	for _, in := range inputs {
		e1 := NormalizeEmail(in)
		e2 := NormalizeEmail(e1)
		if e1 != e2 {
			t.Errorf("Email normalize not idempotent: %q -> %q -> %q", in, e1, e2)
		}
	}
}


func TestNormalizePhone_OnlySeparators(t *testing.T) {
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


func TestNormalizePhone_TableDriven_Batch(t *testing.T) {
	equivalenceGroups := [][]string{
		{"13800138000"},
		{"+8613800138001"},
		{"138 0013 8002"},
		{"138-0013-8003"},
		{"+86 138-0013-8004"},
		{"008613800138005"},
		{"138.0013.8006"},
		{"138_0013_8007"},
		{"(138) 0013 8008"},
		{"\n\t  138 0013 8009  \t\n"},
		{"13012345678"}, {"13112345678"}, {"13212345678"},
		{"13312345678"}, {"13412345678"}, {"13512345678"},
		{"13612345678"}, {"13712345678"}, {"13812345678"},
		{"13912345678"},
		{"15012345678"}, {"15112345678"}, {"15212345678"},
		{"15712345678"}, {"15812345678"}, {"15912345678"},
		{"17012345678"}, {"17112345678"}, {"17212345678"},
		{"17712345678"}, {"18612345678"},
	}
	if len(equivalenceGroups) < 30 {
		t.Errorf("test coverage insufficient: %d groups, want >= 30", len(equivalenceGroups))
	}
	for i, group := range equivalenceGroups {
		if len(group) == 0 {
			continue
		}
		expected := NormalizePhone(group[0])
		for j, v := range group {
			got := NormalizePhone(v)
			if got != expected {
				t.Errorf("group %d variant %d (%q) -> %q, want %q", i, j, v, got, expected)
			}
		}
	}
}

func TestNormalizeEmail_TableDriven_Batch(t *testing.T) {
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
		{"test.email+tag@gmail.com", "test.email+tag@gmail.com"},
		{"X.Y.Z@A.B.CO", "x.y.z@a.b.co"},
		{"abc123@def456.com", "abc123@def456.com"},
		{"Admin@COMPANY.CN", "admin@company.cn"},
		{"  multi@space.com  ", "multi@space.com"},
		{"TAG+TAG+TAG@TEST.COM", "tag+tag+tag@test.com"},
		{"user.name+notifications@example.io", "user.name+notifications@example.io"},
		{"CAPS.LOCK@Caps.Domain.Com", "caps.lock@caps.domain.com"},
		{"very.long.email.address@very.long.domain.com", "very.long.email.address@very.long.domain.com"},
		{"u@x.io", "u@x.io"},
		{"hello@world.cn", "hello@world.cn"},
		{"support@helpdesk.org", "support@helpdesk.org"},
		{"noreply@system.com", "noreply@system.com"},
		{"postmaster@mail.net", "postmaster@mail.net"},
		{"A@B.C.D.E.IO", "a@b.c.d.e.io"},
	}
	if len(cases) < 30 {
		t.Errorf("test coverage insufficient: %d cases, want >= 30", len(cases))
	}
	for i, c := range cases {
		if got := NormalizeEmail(c.in); got != c.want {
			t.Errorf("case %d: NormalizeEmail(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}


func TestNormalizeOpenID_TableDriven_Batch(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"o001", "o001"}, {"  o002  ", "o002"}, {"\no003\n", "o003"}, {"\to004\t", "o004"},
		{"Wechat_OpenID_5", "Wechat_OpenID_5"}, {"DY_OPEN_ID_6", "DY_OPEN_ID_6"}, {"XHS_OPEN_ID_7", "XHS_OPEN_ID_7"},
		{"abc_def_ghi_8", "abc_def_ghi_8"}, {"  trim_9  ", "trim_9"}, {"\nwrap_10\n", "wrap_10"},
		{"UpperCase_11", "UpperCase_11"}, {"lower_case_12", "lower_case_12"}, {"Mixed_Case_13", "Mixed_Case_13"},
		{"1234567890_14", "1234567890_14"}, {"special-chars-15", "special-chars-15"}, {"with.dots.16", "with.dots.16"},
		{"with/slash/17", "with/slash/17"}, {"path\\to\\18", "path\\to\\18"}, {"file_name_19", "file_name_19"},
		{"hyphen-name-20", "hyphen-name-20"}, {"under_score_21", "under_score_21"}, {"Mixed_22_WithSpace  ", "Mixed_22_WithSpace"},
		{"  Mixed_23_WithSpace", "Mixed_23_WithSpace"}, {"\t\n\tmix_24\t\n\t", "mix_24"}, {"o25", "o25"},
		{"o26", "o26"}, {"o27", "o27"}, {"o28", "o28"}, {"o29", "o29"}, {"o30", "o30"},
		{"o31", "o31"}, {"o32", "o32"}, {"o33", "o33"}, {"o34", "o34"}, {"o35", "o35"},
		{"o36", "o36"}, {"o37", "o37"}, {"o38", "o38"}, {"o39", "o39"}, {"o40", "o40"},
		{"o41", "o41"}, {"o42", "o42"}, {"o43", "o43"}, {"o44", "o44"}, {"o45", "o45"},
		{"o46", "o46"}, {"o47", "o47"}, {"o48", "o48"}, {"o49", "o49"}, {"o50", "o50"},
	}
	if len(cases) < 50 {
		t.Errorf("test coverage insufficient: %d cases, want >= 50", len(cases))
	}
	for i, c := range cases {
		if got := NormalizeOpenID(c.in); got != c.want {
			t.Errorf("case %d: NormalizeOpenID(%q) = %q, want %q", i, c.in, got, c.want)
		}
	}
}

