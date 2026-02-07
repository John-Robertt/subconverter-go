package ss

import "testing"

func FuzzParseSubscriptionText(f *testing.F) {
	seed := []string{
		"",
		"   \n",
		"# comment\nss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388#Node%201\n",
		"ss://YWVzLTEyOC1nY206cGFzc3dvcmQ=@example.com:8388#A\n",
		"ss://YWVzLTEyOC1nY206cGFzcw==@example.com:8388/?plugin=simple-obfs%3Bobfs%3Dtls%3Bobfs-host%3Dexample.com#obfs\n",
		"ss://YWVzLTEyOC1nY206cGFzcw==@[::1]:8388#ipv6\n",
	}
	for _, s := range seed {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, content string) {
		proxies, err := ParseSubscriptionText("https://example.com/sub", content)
		if err != nil {
			return
		}

		if len(proxies) == 0 {
			t.Fatalf("proxies is empty on nil error")
		}
		for _, p := range proxies {
			if p.Type != "ss" {
				t.Fatalf("unexpected proxy type: %q", p.Type)
			}
			if p.Server == "" {
				t.Fatalf("empty server")
			}
			if p.Port < 1 || p.Port > 65535 {
				t.Fatalf("port out of range: %d", p.Port)
			}
			if p.Cipher == "" {
				t.Fatalf("empty cipher")
			}
			if p.Password == "" {
				t.Fatalf("empty password")
			}
			for _, kv := range p.PluginOpts {
				if kv.Key == "" {
					t.Fatalf("empty plugin option key")
				}
			}
		}
	})
}
