// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cloudinit

import (
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

type renderedUser struct {
	Name       string   `yaml:"name"`
	Sudo       string   `yaml:"sudo"`
	Groups     []string `yaml:"groups"`
	LockPasswd bool     `yaml:"lock_passwd"`
}

type renderedUserData struct {
	Groups   []string `yaml:"groups"`
	Users    []any    `yaml:"users"`
	Chpasswd struct {
		Users []struct {
			Name string `yaml:"name"`
			Type string `yaml:"type"`
		} `yaml:"users"`
	} `yaml:"chpasswd"`
}

func renderUserData(t *testing.T, cfg Config) (renderedUserData, renderedUser) {
	t.Helper()
	seed, err := Build(cfg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var doc renderedUserData
	if err := yaml.Unmarshal(seed.UserData, &doc); err != nil {
		t.Fatalf("user-data is not YAML: %v\n%s", err, seed.UserData)
	}
	if len(doc.Users) != 2 {
		t.Fatalf("users = %v, want the image default plus the wizard's account", doc.Users)
	}
	raw, err := yaml.Marshal(doc.Users[1])
	if err != nil {
		t.Fatal(err)
	}
	var account renderedUser
	if err := yaml.Unmarshal(raw, &account); err != nil {
		t.Fatal(err)
	}
	return doc, account
}

// TestWizardAccountUsesNoDistroSpecificGroup is the AlmaLinux failure: a `sudo`
// group exists only on Debian and Ubuntu, useradd refuses an unknown group, and
// cloud-init then never creates the account the wizard's password belongs to.
func TestWizardAccountUsesNoDistroSpecificGroup(t *testing.T) {
	doc, account := renderUserData(t, Config{
		InstanceID:   "db242992stfb1bk2totg",
		Hostname:     "mcawesome-dev",
		DefaultUser:  "carlo",
		PasswordHash: "$6$saltsalt$hashhashhash",
	})

	for _, group := range account.Groups {
		if group == "sudo" || group == "wheel" {
			t.Fatalf("account groups %v name %q, which RHEL-family or Debian-family images lack", account.Groups, group)
		}
	}
	for _, group := range account.Groups {
		if !slices.Contains(doc.Groups, group) {
			t.Fatalf("account group %q is not created by the top-level groups list %v", group, doc.Groups)
		}
	}
	if account.Name != "carlo" || account.Sudo != "ALL=(ALL) NOPASSWD:ALL" || account.LockPasswd {
		t.Fatalf("account = %+v, want carlo with the sudoers rule and an unlocked password", account)
	}
	if len(doc.Chpasswd.Users) != 1 || doc.Chpasswd.Users[0].Name != "carlo" || doc.Chpasswd.Users[0].Type != "hash" {
		t.Fatalf("chpasswd = %+v, want the hashed password for carlo", doc.Chpasswd)
	}
}
