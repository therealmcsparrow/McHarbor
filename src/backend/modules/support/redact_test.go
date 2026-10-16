// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"strings"
	"testing"
)

// TestEnvAllowListFailsClosed keeps a variable that is not explicitly safe from ever printing
// its value, including a secret someone adds to the allow-list by mistake.
func TestEnvAllowListFailsClosed(t *testing.T) {
	env := map[string]string{
		"PORT": "8705", "ENCRYPTION_KEY": "k-value-123456", "MCHARBOR_NEW_THING": "n-value-123456",
		"DOCKER_HOST": "unix:///var/run/docker.sock", "AUTH_DISABLE": "false",
	}
	safeEnvKeys["MCHARBOR_API_TOKEN"] = true
	defer delete(safeEnvKeys, "MCHARBOR_API_TOKEN")
	env["MCHARBOR_API_TOKEN"] = "t-value-123456"
	var environ []string
	for k, v := range env {
		environ = append(environ, k+"="+v)
	}
	got := map[string]EnvEntry{}
	for _, e := range RedactedEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }, environ, NewScrubber()) {
		got[e.Name] = e
	}
	if got["PORT"].Value != "8705" || got["AUTH_DISABLE"].Value != "false" {
		t.Errorf("allow-listed values were withheld: %+v %+v", got["PORT"], got["AUTH_DISABLE"])
	}
	for _, name := range []string{"ENCRYPTION_KEY", "MCHARBOR_NEW_THING", "DOCKER_HOST", "MCHARBOR_API_TOKEN"} {
		if e := got[name]; !e.Set || e.Value != Redacted {
			t.Errorf("%s = %+v, want set and redacted", name, e)
		}
	}
	if e := got["MCHARBOR_SECRET"]; e.Set || e.Value != "" {
		t.Errorf("an unset variable reads as set: %+v", e)
	}
}

// TestScrubberRemovesCredentialShapes covers the shapes credentials take in logs and errors
// when nobody told the scrubber the literal value.
func TestScrubberRemovesCredentialShapes(t *testing.T) {
	s := NewScrubber("literal-known-secret")
	cases := map[string]string{
		"postgres://user:pgpass123@db:5432/x":                          "pgpass123",
		"root:mysqlpass9@tcp(db:3306)/mch":                             "mysqlpass9",
		"Authorization: Bearer abcdefghijklmnop":                       "abcdefghijklmnop",
		`{"password": "jsonpass77"}`:                                   "jsonpass77",
		"api_key=keyvalue55 next":                                      "keyvalue55",
		"https://h/x?token=querytok66&a=1":                             "querytok66",
		"cipher enc:v1:QUJDREVGR0g= done":                              "QUJDREVGR0g=",
		"-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----": "MIIB",
		"loaded literal-known-secret from disk":                        "literal-known-secret",
		"b64 bGl0ZXJhbC1rbm93bi1zZWNyZXQ= here":                        "bGl0ZXJhbC1rbm93bi1zZWNyZXQ=",
	}
	for in, secret := range cases {
		if out := s.Scrub(in); strings.Contains(out, secret) {
			t.Errorf("Scrub(%q) = %q still contains %q", in, out, secret)
		}
	}
	if out := s.Scrub("plain status line 200 OK"); out != "plain status line 200 OK" {
		t.Errorf("harmless text was altered: %q", out)
	}
}
