// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package redact

import (
	"strings"
	"testing"
)

func TestErrorDetailCutsSecretsAndKeepsCause(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		mustNot []string
		must    []string
	}{
		{
			name:    "socket path",
			in:      "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
			mustNot: []string{"/var/run/docker.sock"},
			must:    []string{"Cannot connect to the Docker daemon", "[socket]"},
		},
		{
			name:    "url credentials",
			in:      `Get "https://admin:hunter2@registry.example.com/v2/": dial tcp 10.0.0.5:443: connect: connection refused`,
			mustNot: []string{"hunter2"},
			must:    []string{"10.0.0.5:443", "connection refused", "registry.example.com"},
		},
		{
			name:    "bearer header",
			in:      "upstream rejected Authorization: Bearer abcdefghijklmnop.qrstuv",
			mustNot: []string{"abcdefghijklmnop"},
			must:    []string{"upstream rejected"},
		},
		{
			name:    "key value pairs",
			in:      `invalid config: password=s3cr3t api_key: "xyz123" "client_secret":"abc" host=db1`,
			mustNot: []string{"s3cr3t", "xyz123", `"abc"`},
			must:    []string{"password=", "host=db1"},
		},
		{
			name:    "pem block",
			in:      "tls: bad key -----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBg\n-----END PRIVATE KEY----- end",
			mustNot: []string{"MIIEvQIBADANBg"},
			must:    []string{"tls: bad key", "[REDACTED:pem]", "end"},
		},
		{
			name:    "agent token",
			in:      "agent auth failed for agt_0123456789abcdef0123456789abcdef",
			mustNot: []string{"0123456789abcdef0123456789abcdef"},
			must:    []string{"agent auth failed"},
		},
		{
			name: "plain docker error kept intact",
			in:   "Error response from daemon: driver failed programming external connectivity on endpoint web: Bind for 0.0.0.0:8080 failed: port is already allocated",
			must: []string{"port is already allocated", "0.0.0.0:8080", "endpoint web"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ErrorDetail(tc.in)
			for _, s := range tc.mustNot {
				if strings.Contains(got, s) {
					t.Errorf("ErrorDetail kept %q: %s", s, got)
				}
			}
			for _, s := range tc.must {
				if !strings.Contains(got, s) {
					t.Errorf("ErrorDetail dropped %q: %s", s, got)
				}
			}
		})
	}
}

func TestErrorDetailCapsLength(t *testing.T) {
	got := ErrorDetail(strings.Repeat("é", ErrorDetailMaxLen))
	if len(got) > ErrorDetailMaxLen+len(" …") {
		t.Fatalf("detail not capped: %d bytes", len(got))
	}
	if !strings.HasSuffix(got, " …") {
		t.Fatalf("capped detail has no ellipsis")
	}
	if strings.ContainsRune(got, '�') {
		t.Fatalf("cap split a rune")
	}
}
