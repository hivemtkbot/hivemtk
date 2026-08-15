package urlguard


import (
	"strings"
	"testing"
)

func TestValidateURL_Scheme(t *testing.T) {
	cases := []struct {
		url   string
		valid bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"HTTP://Example.COM", true}, 
		{"file:///etc/passwd", false},
		{"gopher://127.0.0.1:6379/_INFO", false},
		{"ftp://example.com/", false},
		{"dict://localhost:11211/stats", false},
		{"javascript:alert(1)", false},
	}
	for _, c := range cases {
		err := ValidateURL(c.url)
		if c.valid && err != nil {
			t.Errorf("expected %q to pass, got error: %v", c.url, err)
		}
		if !c.valid && err == nil {
			t.Errorf("expected %q to be blocked, but it passed", c.url)
		}
		if !c.valid && err != nil && !strings.Contains(err.Error(), "scheme") && !strings.Contains(err.Error(), "host") && !strings.Contains(err.Error(), "private") {
		}
	}
}

func TestValidateURL_PrivateIPs(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/",
		"http://127.1.2.3/",
		"http://localhost/", 
		"http://10.0.0.1/",
		"http://10.255.255.255/",
		"http://172.16.0.1/",
		"http://172.31.255.255/",
		"http://192.168.1.1/",
		"http://192.168.0.0/",
		"http://169.254.169.254/", 
		"http://169.254.0.1/",
		"http://0.0.0.0/",
		"http://100.64.0.1/", 
		"http://100.127.255.255/",
		"http://[::1]/",              
		"http://[fe80::1]/",          
		"http://[fc00::1]/",          
		"http://[fd00::1]/",          
		"http://[::ffff:127.0.0.1]/", 
		"http://[::ffff:10.0.0.1]/",  
	}
	for _, u := range blocked {
		err := ValidateURL(u)
		if err == nil {
			t.Errorf("expected %q to be blocked, but it passed", u)
		}
	}
}

func TestValidateURL_PublicIPs(t *testing.T) {
	cases := []string{
		"http://8.8.8.8/",
		"http://1.1.1.1/",
		"http://203.0.113.1/", 
	}
	for _, u := range cases {
		err := ValidateURL(u)
		if err != nil {
			t.Errorf("expected %q to pass, got error: %v", u, err)
		}
	}
}

func TestValidateURL_EmptyAndInvalid(t *testing.T) {
	invalid := []string{
		"",
		"http://",
		"http:///",
		"://missing-scheme",
	}
	for _, u := range invalid {
		err := ValidateURL(u)
		if err == nil {
			t.Errorf("expected %q to fail, but it passed", u)
		}
	}
}

