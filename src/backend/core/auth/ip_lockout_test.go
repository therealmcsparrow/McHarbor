// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package auth

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/httpx"
)

const fixturePassword = "correct horse battery staple"

func newIPLockoutService(t *testing.T, maxPerIP int) (*Service, *sql.DB) {
	t.Helper()
	svc, database := newSudoTestService(t)
	if _, err := database.Exec(`UPDATE settings SET value = ? WHERE key = ?`,
		fmt.Sprint(maxPerIP), SettingLockoutIPMaxAttempts); err != nil {
		t.Fatalf("setting per-address threshold: %v", err)
	}
	return svc, database
}

func mustLogin(t *testing.T, svc *Service, username, password string, client LoginClient) *AuthResult {
	t.Helper()
	result, err := svc.LoginFrom(username, password, client)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return result
}

func countIPLocks(t *testing.T, database *sql.DB) int {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM login_ip_locks`).Scan(&n); err != nil {
		t.Fatalf("counting locks: %v", err)
	}
	return n
}

func TestMigrationSeedsIPLockoutDefaults(t *testing.T) {
	_, database := newSudoTestService(t)
	policy := LoadLockoutPolicy(t.Context(), database)
	want := DefaultLockoutPolicy()
	if policy.MaxAttemptsPerIP != want.MaxAttemptsPerIP || policy.IPWindow != want.IPWindow || policy.IPLock != want.IPLock {
		t.Fatalf("seeded policy %+v does not match defaults %+v", policy, want)
	}
}

func TestIPLockoutLocksSprayingAddress(t *testing.T) {
	svc, _ := newIPLockoutService(t, 3)
	attacker := LoginClient{IP: "203.0.113.5", Source: httpx.ClientAddressSourcePeer}

	for i := 1; i <= 2; i++ {
		res := mustLogin(t, svc, fmt.Sprintf("nobody%d", i), "guess", attacker)
		if res.Locked {
			t.Fatalf("attempt %d locked before the threshold", i)
		}
	}
	if res := mustLogin(t, svc, "nobody3", "guess", attacker); !res.Locked || res.RetryAfterSeconds <= 0 {
		t.Fatalf("third failure across distinct usernames did not lock the address: %+v", res)
	}
	if res := mustLogin(t, svc, "alice", fixturePassword, attacker); res.Success || !res.Locked {
		t.Fatalf("a locked address still logged in with a valid password: %+v", res)
	}
	if res := mustLogin(t, svc, "alice", fixturePassword, LoginClient{IP: "198.51.100.20"}); !res.Success {
		t.Fatalf("another address was caught by the first address's lock: %+v", res)
	}
}

func TestIPLockoutNeverCountsEmptyAddress(t *testing.T) {
	for _, ip := range []string{"", "unknown"} {
		svc, database := newIPLockoutService(t, 3)
		for i := 0; i < 10; i++ {
			if res := mustLogin(t, svc, fmt.Sprintf("nobody%d", i), "guess", LoginClient{IP: ip}); res.Locked {
				t.Fatalf("unattributed address %q was locked on attempt %d", ip, i+1)
			}
		}
		if n := countIPLocks(t, database); n != 0 {
			t.Fatalf("unattributed address %q produced %d lock rows", ip, n)
		}
	}
}

func TestIPLockoutNeverCountsTrustedProxy(t *testing.T) {
	svc, database := newIPLockoutService(t, 3)
	proxies, err := httpx.NewClientAddressResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	httpx.SetDefaultResolver(proxies)
	t.Cleanup(func() { httpx.SetDefaultResolver(nil) })

	for i := 0; i < 10; i++ {
		if res := mustLogin(t, svc, fmt.Sprintf("nobody%d", i), "guess", LoginClient{IP: "10.1.2.3"}); res.Locked {
			t.Fatalf("the trusted proxy address was locked on attempt %d", i+1)
		}
	}
	if n := countIPLocks(t, database); n != 0 {
		t.Fatalf("trusted proxy produced %d lock rows", n)
	}
}

func TestIPLockoutResponseMatchesAccountLockout(t *testing.T) {
	accountSvc, _ := newIPLockoutService(t, 0)
	var accountLock *AuthResult
	for i := 0; i < DefaultLockoutPolicy().MaxAttempts; i++ {
		accountLock = mustLogin(t, accountSvc, "alice", "wrong", LoginClient{IP: fmt.Sprintf("198.51.100.%d", i+1)})
	}
	if !accountLock.Locked {
		t.Fatalf("account did not lock: %+v", accountLock)
	}

	ipSvc, _ := newIPLockoutService(t, 2)
	client := LoginClient{IP: "203.0.113.9"}
	mustLogin(t, ipSvc, "nobody", "guess", client)
	ipLock := mustLogin(t, ipSvc, "nobody", "guess", client)
	if !ipLock.Locked {
		t.Fatalf("address did not lock: %+v", ipLock)
	}

	if accountLock.RetryAfterSeconds <= 0 || ipLock.RetryAfterSeconds <= 0 {
		t.Fatalf("both locks must carry a retry-after: account %d, address %d",
			accountLock.RetryAfterSeconds, ipLock.RetryAfterSeconds)
	}
	a, b := *accountLock, *ipLock
	a.RetryAfterSeconds, b.RetryAfterSeconds = 0, 0
	if a != b {
		t.Fatalf("address lock is distinguishable from account lock:\naccount %+v\naddress %+v", a, b)
	}
}

func TestLoginRecordsAttributionSource(t *testing.T) {
	svc, database := newIPLockoutService(t, 30)
	mustLogin(t, svc, "alice", "wrong", LoginClient{IP: "203.0.113.4", Source: httpx.ClientAddressSourceXForwardedFor})

	var ip, source string
	if err := database.QueryRow(`SELECT ip_address, ip_source FROM login_attempts`).Scan(&ip, &source); err != nil {
		t.Fatalf("reading attempt: %v", err)
	}
	if ip != "203.0.113.4" || source != httpx.ClientAddressSourceXForwardedFor {
		t.Fatalf("recorded %q via %q", ip, source)
	}
}
