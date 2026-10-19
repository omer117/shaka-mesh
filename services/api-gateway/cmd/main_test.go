package main

import "testing"

func TestRateLimitFromEnvValidation(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{"defaults", nil, false},
		{"trusted proxy", map[string]string{"TRUSTED_PROXY_HOPS": "1"}, false},
		{"negative hops", map[string]string{"TRUSTED_PROXY_HOPS": "-1"}, true},
		{"not a number", map[string]string{"AUTH_RATE_LIMIT_IP_BURST": "lots"}, true},
		{"zero burst", map[string]string{"AUTH_RATE_LIMIT_EMAIL_BURST": "0"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range []string{
				"AUTH_RATE_LIMIT_IP_BURST", "AUTH_RATE_LIMIT_IP_PER_MINUTE",
				"AUTH_RATE_LIMIT_EMAIL_BURST", "AUTH_RATE_LIMIT_EMAIL_PER_MINUTE", "TRUSTED_PROXY_HOPS",
			} {
				t.Setenv(name, tc.env[name])
			}
			_, err := rateLimitFromEnv()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
