package rules

import "testing"

func FuzzParseInlineRule(f *testing.F) {
	seed := []string{
		"",
		"  \n",
		"# comment",
		"MATCH,DIRECT",
		"DOMAIN,example.com,DIRECT",
		"DOMAIN-SUFFIX,example.com,PROXY",
		"DOMAIN-KEYWORD,google,REJECT",
		"GEOIP,CN,DIRECT",
		"PROCESS-NAME,WeChat,PROXY",
		"URL-REGEX,^https?://,PROXY",
		"IP-CIDR,1.2.3.0/24,DIRECT",
		"IP-CIDR,1.2.3.0/24,DIRECT,no-resolve",
		"IP-CIDR6,2001:db8::/32,REJECT",
		"IP-CIDR6,2001:db8::/32,REJECT,no-resolve",
	}
	for _, s := range seed {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, line string) {
		r, err := ParseInlineRule(line)
		if err != nil {
			return
		}

		if r.Type == "" {
			t.Fatalf("empty rule type")
		}
		if r.Action == "" {
			t.Fatalf("empty rule action")
		}
		if r.Type != "MATCH" && r.Value == "" {
			t.Fatalf("empty rule value for type=%q", r.Type)
		}
		if r.NoResolve && r.Type != "IP-CIDR" && r.Type != "IP-CIDR6" {
			t.Fatalf("no-resolve on non-cidr rule: type=%q", r.Type)
		}
	})
}
