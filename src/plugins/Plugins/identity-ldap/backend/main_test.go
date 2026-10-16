// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// recordingRegistry captures what RegisterRoutes hands the host, by auth class.
type recordingRegistry struct {
	calls []string
}

// Public records a route registered with no rate limit or session.
func (r *recordingRegistry) Public(method, path string, _ http.HandlerFunc) {
	r.calls = append(r.calls, "public "+method+" "+path)
}

// Auth records a route registered behind the host login rate limit.
func (r *recordingRegistry) Auth(method, path string, _ http.HandlerFunc) {
	r.calls = append(r.calls, "auth "+method+" "+path)
}

// Protected records a route registered behind the session middleware.
func (r *recordingRegistry) Protected(method, path string, _ http.HandlerFunc) {
	r.calls = append(r.calls, "protected "+method+" "+path)
}

// TestAuthRouteLeavesTheHostIdentitySubtree guards the shipped 404: the login
// route sat under /api/identity-providers, which the host's subrouter owns.
func TestAuthRouteLeavesTheHostIdentitySubtree(t *testing.T) {
	if ldapAuthPath != "/api/identity-ldap/auth" {
		t.Fatalf("ldapAuthPath = %q; the login page and docs name /api/identity-ldap/auth", ldapAuthPath)
	}
	if strings.HasPrefix(ldapAuthPath, "/api/identity-providers/") {
		t.Fatalf("%s is inside the host's /api/identity-providers subtree and answers 404", ldapAuthPath)
	}

	reg := &recordingRegistry{}
	p := &IdentityLdapPlugin{}
	if err := p.RegisterRoutes(reg); err != nil {
		t.Fatalf("RegisterRoutes: %v", err)
	}
	want := "auth " + http.MethodPost + " " + ldapAuthPath
	if len(reg.calls) != 1 || reg.calls[0] != want {
		t.Fatalf("RegisterRoutes registered %v; want exactly [%s] so the host login rate limit applies", reg.calls, want)
	}
}

// TestManifestDeclaresTheAuthRoutePublic keeps plugin.yaml and the code in
// step: the loader mounts only what the manifest declares, and without
// public: true the Auth-class route would demand a permission nobody holds.
func TestManifestDeclaresTheAuthRoutePublic(t *testing.T) {
	raw, err := os.ReadFile("../plugin.yaml")
	if err != nil {
		t.Fatalf("read plugin.yaml: %v", err)
	}
	var entry string
	for _, block := range strings.Split(string(raw), "- method:") {
		if strings.Contains(block, `"`+ldapAuthPath+`"`) {
			entry = block
			break
		}
	}
	if entry == "" {
		t.Fatalf("plugin.yaml declares no route at %s", ldapAuthPath)
	}
	if !strings.HasPrefix(strings.TrimSpace(entry), "POST") {
		t.Errorf("plugin.yaml declares %s with a method other than POST", ldapAuthPath)
	}
	if !strings.Contains(entry, "public: true") {
		t.Errorf("plugin.yaml does not declare %s public: true", ldapAuthPath)
	}
	if strings.Contains(string(raw), "/api/identity-providers/ldap/auth\"") {
		t.Errorf("plugin.yaml still declares the shadowed /api/identity-providers/ldap/auth route")
	}
}
