// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package httpx

import "testing"

func TestIsTrustedProxy(t *testing.T) {
	t.Cleanup(func() { SetDefaultResolver(nil) })

	SetDefaultResolver(nil)
	if IsTrustedProxy("10.0.0.1") {
		t.Fatal("no resolver must trust nothing")
	}

	resolver, err := NewClientAddressResolver([]string{"10.0.0.0/8", "2001:db8::/32"})
	if err != nil {
		t.Fatal(err)
	}
	SetDefaultResolver(resolver)

	cases := map[string]bool{
		"10.1.2.3":        true,
		"::ffff:10.1.2.3": true,
		"2001:db8::5":     true,
		"203.0.113.1":     false,
		"":                false,
		"unknown":         false,
		"not-an-address":  false,
	}
	for ip, want := range cases {
		if got := IsTrustedProxy(ip); got != want {
			t.Errorf("IsTrustedProxy(%q) = %v, want %v", ip, got, want)
		}
	}
}
