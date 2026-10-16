// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

const testDN = "uid=dana,ou=people,dc=example,dc=test"

type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}
func (nopLogger) Debug(string, ...any) {}

// plainIdentity is a host.Identity from a host that predates external login.
type plainIdentity struct{}

func (plainIdentity) ProvisionUser(context.Context, plugin.ProvisionRequest) (*plugin.ProvisionedUser, error) {
	return nil, errors.New("plugins must not provision directly")
}

// fakeHost records the order of host calls and writes a marker response for each
// terminal one, so a test can tell who answered.
type fakeHost struct {
	plainIdentity
	lockAddress bool
	lockAccount bool
	calls       []string
	failReason  string
	failDN      string
	completed   []string
}

func (h *fakeHost) LDAPProviderConfig(_ context.Context, providerID string) (map[string]string, error) {
	h.calls = append(h.calls, "config")
	if providerID != "prov-1" {
		return nil, errors.New("no such provider")
	}
	return map[string]string{"url": "ldap://directory.test", "user_base_dn": "ou=people,dc=example,dc=test", "bind_password": "svc-secret"}, nil
}

func (h *fakeHost) BeginExternalLogin(_ context.Context, w http.ResponseWriter, _ *http.Request, _, _ string) error {
	h.calls = append(h.calls, "begin")
	if h.lockAddress {
		w.WriteHeader(http.StatusLocked)
		return errors.New("refused")
	}
	return nil
}

func (h *fakeHost) CheckExternalAccount(_ context.Context, w http.ResponseWriter, _ *http.Request, _, _, externalID, _ string) error {
	h.calls = append(h.calls, "check:"+externalID)
	if h.lockAccount {
		w.WriteHeader(http.StatusLocked)
		return errors.New("refused")
	}
	return nil
}

func (h *fakeHost) FailExternalLogin(_ context.Context, w http.ResponseWriter, _ *http.Request, _, _, externalID, reason string) {
	h.calls = append(h.calls, "fail")
	h.failReason, h.failDN = reason, externalID
	w.WriteHeader(http.StatusUnauthorized)
}

func (h *fakeHost) CompleteExternalLogin(_ context.Context, w http.ResponseWriter, _ *http.Request, providerID, externalID, username, email, displayName string, groups []string) error {
	h.calls = append(h.calls, "complete")
	h.completed = append([]string{providerID, externalID, username, email, displayName}, groups...)
	w.WriteHeader(http.StatusOK)
	return nil
}

// fakeDirectory counts every bind the plugin attempts.
type fakeDirectory struct {
	opens        int
	userBinds    int
	password     string
	missing      bool
	unreachable  bool
	seenBindPass string
}

func (d *fakeDirectory) Open(prov ldapProviderConfig) (ldapSession, error) {
	d.opens++
	d.seenBindPass = prov.BindPassword
	if d.unreachable {
		return nil, errors.New("dial tcp: connection refused")
	}
	return &fakeSession{dir: d}, nil
}

type fakeSession struct{ dir *fakeDirectory }

func (s *fakeSession) FindUser(string) (*ldapEntry, error) {
	if s.dir.missing {
		return nil, errDirectoryUserNotFound
	}
	return &ldapEntry{DN: testDN, Username: "dana", Email: "dana@example.test", Name: "Dana", Groups: []string{"ops"}}, nil
}

func (s *fakeSession) Authenticate(dn, password string) error {
	s.dir.userBinds++
	if dn != testDN || password != s.dir.password {
		return errDirectoryInvalidCredentials
	}
	return nil
}

func (s *fakeSession) Close() {}

func newFlowPlugin(identity plugin.IdentityProvisioner, dir *fakeDirectory) *IdentityLdapPlugin {
	return &IdentityLdapPlugin{host: plugin.HostContext{Logger: nopLogger{}, Identity: identity}, dir: dir}
}

func postAuth(p *IdentityLdapPlugin, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	p.handleAuth(rec, httptest.NewRequest(http.MethodPost, ldapAuthPath, strings.NewReader(body)))
	return rec
}

const goodBody = `{"providerId":"prov-1","username":"dana","password":"right"}`

func TestLockedAddressIsRefusedBeforeAnyBind(t *testing.T) {
	host := &fakeHost{lockAddress: true}
	dir := &fakeDirectory{password: "right"}
	rec := postAuth(newFlowPlugin(host, dir), goodBody)
	if rec.Code != http.StatusLocked {
		t.Fatalf("status = %d; want the host's 423", rec.Code)
	}
	if dir.opens != 0 || dir.userBinds != 0 {
		t.Fatalf("a locked address reached the directory: %d opens, %d binds", dir.opens, dir.userBinds)
	}
	if strings.Join(host.calls, ",") != "begin" {
		t.Fatalf("host calls %v; want only the address gate", host.calls)
	}
}

func TestLockedAccountIsRefusedBeforeTheUserBind(t *testing.T) {
	host := &fakeHost{lockAccount: true}
	dir := &fakeDirectory{password: "right"}
	rec := postAuth(newFlowPlugin(host, dir), goodBody)
	if rec.Code != http.StatusLocked || dir.userBinds != 0 {
		t.Fatalf("status %d after %d user binds; want 423 with no bind", rec.Code, dir.userBinds)
	}
}

func TestFailedBindIsReportedToTheHostWithTheEntry(t *testing.T) {
	host := &fakeHost{}
	dir := &fakeDirectory{password: "right"}
	rec := postAuth(newFlowPlugin(host, dir), `{"providerId":"prov-1","username":"dana","password":"wrong"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d; want the host's refusal", rec.Code)
	}
	if host.failReason != reasonInvalidCredentials || host.failDN != testDN {
		t.Fatalf("FailExternalLogin(%q, %q); want invalid_credentials against %s so the account counter moves", host.failReason, host.failDN, testDN)
	}
	if got := strings.Join(host.calls, ","); got != "begin,config,check:"+testDN+",fail" {
		t.Fatalf("host call order %s", got)
	}
}

func TestUnknownUserCountsAndOutageDoesNot(t *testing.T) {
	host := &fakeHost{}
	postAuth(newFlowPlugin(host, &fakeDirectory{missing: true}), goodBody)
	if host.failReason != reasonInvalidCredentials || host.failDN != "" {
		t.Fatalf("unknown user reported as (%q, %q)", host.failReason, host.failDN)
	}
	outage := &fakeHost{}
	postAuth(newFlowPlugin(outage, &fakeDirectory{unreachable: true}), goodBody)
	if outage.failReason != reasonUnavailable {
		t.Fatalf("an unreachable directory reported %q; it must not count toward lockout", outage.failReason)
	}
}

func TestSuccessIsCompletedByTheHost(t *testing.T) {
	host := &fakeHost{}
	dir := &fakeDirectory{password: "right"}
	rec := postAuth(newFlowPlugin(host, dir), goodBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	want := "prov-1," + testDN + ",dana,dana@example.test,Dana,ops"
	if got := strings.Join(host.completed, ","); got != want {
		t.Fatalf("CompleteExternalLogin(%s); want %s", got, want)
	}
	if dir.seenBindPass != "svc-secret" {
		t.Fatalf("the directory was not opened with the host-decrypted config")
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("the plugin set a cookie itself; sessions are the host's")
	}
}

func TestPluginFailsClosedOnAnOlderHost(t *testing.T) {
	dir := &fakeDirectory{password: "right"}
	rec := postAuth(newFlowPlugin(plainIdentity{}, dir), goodBody)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; want 503 from a host without the capability", rec.Code)
	}
	if dir.opens != 0 {
		t.Fatal("the plugin contacted the directory without a host to account for it")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body.String(), err)
	}
	if _, ok := body["error"].(string); !ok {
		t.Fatalf("error is %T; the login page renders it as text, so it must be a string", body["error"])
	}
}

func TestEmptyPasswordNeverReachesTheDirectory(t *testing.T) {
	host := &fakeHost{}
	dir := &fakeDirectory{password: ""}
	rec := postAuth(newFlowPlugin(host, dir), `{"providerId":"prov-1","username":"dana","password":""}`)
	if rec.Code != http.StatusBadRequest || dir.opens != 0 || len(host.calls) != 0 {
		t.Fatalf("empty password: status %d, %d opens, host calls %v", rec.Code, dir.opens, host.calls)
	}
}
