// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package identity

import (
	"testing"
)

func TestExternalProviderConfigDecryptsOnlyAnEnabledLDAPProvider(t *testing.T) {
	svc, database := openIdentityTestDB(t)
	ciphertext, err := svc.encryption.Encrypt("s3cret-bind")
	if err != nil {
		t.Fatal(err)
	}
	cfg := `{"url":"ldap://directory.test","bind_dn":"cn=svc","bind_password":"` + encPrefix + ciphertext + `"}`
	mustExec(t, database, `INSERT INTO identity_providers (id, name, provider_type, enabled, client_id, client_secret, config)
		VALUES ('p-ldap', 'Corp LDAP', 'ldap', 1, '', '', ?), ('p-off', 'Old LDAP', 'ldap', 0, '', '', ?), ('p-saml', 'Corp SAML', 'saml_2_0', 1, '', '', '{}')`, cfg, cfg)
	host := NewHostProvisioner(database, svc.encryption, nil)

	got, err := host.ExternalProviderConfig(t.Context(), "p-ldap", "ldap")
	if err != nil {
		t.Fatalf("ExternalProviderConfig: %v", err)
	}
	if got["bind_password"] != "s3cret-bind" || got["url"] != "ldap://directory.test" {
		t.Fatalf("config = %v; want the bind password decrypted host-side", got)
	}
	for _, id := range []string{"p-off", "p-saml", "p-missing"} {
		if _, err := host.ExternalProviderConfig(t.Context(), id, "ldap"); err == nil {
			t.Fatalf("provider %s was handed to the ldap plugin", id)
		}
	}
}

func TestProvisionExternalUserHonoursTheProviderSettings(t *testing.T) {
	svc, database := openIdentityTestDB(t)
	mustExec(t, database, `INSERT INTO identity_providers (id, name, provider_type, enabled, auto_provision, client_id, client_secret)
		VALUES ('p-auto', 'Auto', 'ldap', 1, 1, '', ''), ('p-manual', 'Manual', 'ldap', 1, 0, '', '')`)
	host := NewHostProvisioner(database, svc.encryption, nil)
	dn := "uid=dana,ou=people,dc=example,dc=test"

	userID, err := host.ProvisionExternalUser(t.Context(), "p-manual", dn, "dana", "", "Dana", nil)
	if err != nil || userID != "" {
		t.Fatalf("auto-provision off: got (%q, %v); want no account", userID, err)
	}

	userID, err = host.ProvisionExternalUser(t.Context(), "p-auto", dn, "dana", "", "Dana", nil)
	if err != nil || userID == "" {
		t.Fatalf("auto-provision on: got (%q, %v)", userID, err)
	}
	var username string
	if err := database.QueryRow(`SELECT username FROM users WHERE id = ?`, userID).Scan(&username); err != nil {
		t.Fatal(err)
	}
	if username != "ldap_dana" {
		t.Fatalf("username = %q; want ldap_dana from the login name, not a slice of the DN", username)
	}

	linked, err := host.LinkedUserID(t.Context(), "p-auto", dn, "")
	if err != nil || linked != userID {
		t.Fatalf("LinkedUserID = (%q, %v); want %q so the lockout gate sees the same account", linked, err, userID)
	}
	again, err := host.ProvisionExternalUser(t.Context(), "p-auto", dn, "dana", "", "Dana", nil)
	if err != nil || again != userID {
		t.Fatalf("second sign-in landed on (%q, %v); want the linked %q", again, err, userID)
	}
}
