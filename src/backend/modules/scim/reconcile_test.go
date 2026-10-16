// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package scim

import (
	"reflect"
	"testing"
)

func remoteUser(id, name, email string, active bool) User {
	u := User{ID: id, UserName: name, Active: active}
	if email != "" {
		u.Emails = []Email{{Value: email, Primary: true}}
	}
	return u
}

func changesByScimID(plan SyncPlan) map[string]UserChange {
	out := map[string]UserChange{}
	for _, c := range plan.Users {
		out[c.ScimID] = c
	}
	return out
}

func TestComputePlanClassifiesEveryUser(t *testing.T) {
	locals := []localUser{
		{ID: "u-alice", Username: "alice", Email: "alice@example.com", Active: true},
		{ID: "u-bob", Username: "bob", Email: "bob@old.example.com", Active: true},
		{ID: "u-carol", Username: "carol", Active: true, Protected: true},
		{ID: "u-dave", Username: "dave", Active: true},
		{ID: "u-frank", Username: "frank", Active: true},
		{ID: "u-gina", Username: "gina", Email: "gina@example.com", Active: true},
	}
	mappings := []mappingRow{
		{ID: "m-b", ScimUserID: "b1", McharborUserID: "u-bob", ScimUsername: "bob", ScimEmail: "bob@old.example.com"},
		{ID: "m-c", ScimUserID: "c1", McharborUserID: "u-carol", ScimUsername: "carol"},
		{ID: "m-d", ScimUserID: "d1", McharborUserID: "u-dave", ScimUsername: "dave"},
		{ID: "m-g", ScimUserID: "g1", McharborUserID: "u-gina", ScimUsername: "gina", ScimEmail: "gina@example.com"},
	}
	remote := []User{
		remoteUser("a1", "Alice", "", true),
		remoteUser("b1", "bob", "bob@new.example.com", true),
		remoteUser("c1", "carol", "", false),
		remoteUser("e1", "erin", "erin@example.com", true),
		remoteUser("f1", "frank", "", false),
		remoteUser("g1", "gina", "gina@example.com", true),
		remoteUser("g1", "gina", "gina@example.com", true),
	}
	groups := []Group{{ID: "grp-1", DisplayName: "Operators", Members: []Member{{Value: "a1"}, {Value: "b1"}}}, {ID: "grp-2", DisplayName: "Nobody"}}
	plan := computePlan(remote, groups, mappings, locals, []localGroup{{ID: "lg-ops", Name: "operators"}})

	byID := changesByScimID(plan)
	expect := map[string][2]string{
		"a1": {ActionLink, ""},
		"b1": {ActionUpdate, ""},
		"c1": {ActionSkip, ReasonProtectedAdmin},
		"d1": {ActionDisable, ReasonRemoved},
		"e1": {ActionUnmatched, ""},
		"f1": {ActionDisable, ReasonInactive},
	}
	for id, want := range expect {
		got, ok := byID[id]
		if !ok {
			t.Fatalf("%s: no change in plan %+v", id, plan.Users)
		}
		if got.Action != want[0] || got.Reason != want[1] {
			t.Fatalf("%s: got %s/%s, want %s/%s", id, got.Action, got.Reason, want[0], want[1])
		}
	}
	if _, listed := byID["g1"]; listed {
		t.Fatal("an unchanged linked user must not be listed")
	}
	if !reflect.DeepEqual(byID["b1"].Fields, []string{"email"}) {
		t.Fatalf("bob should change email only, got %v", byID["b1"].Fields)
	}
	if byID["a1"].LocalUserID != "u-alice" || !byID["a1"].Link {
		t.Fatalf("alice must link by case-insensitive username, got %+v", byID["a1"])
	}
	if !byID["f1"].Link {
		t.Fatal("an inactive first sighting still records the link it disables through")
	}
	want := SyncSummary{UsersSeen: 6, Linked: 1, Unmatched: 1, Updated: 1, Disabled: 2, Skipped: 1, Unchanged: 1, GroupsSeen: 2, PendingLink: 1}
	if plan.Summary != want {
		t.Fatalf("summary\n got %+v\nwant %+v", plan.Summary, want)
	}
	if plan.Groups[0].LocalGroupID != "lg-ops" || plan.Groups[0].Members != 2 || plan.Groups[1].LocalGroupID != "" {
		t.Fatalf("groups should match local names case-insensitively, got %+v", plan.Groups)
	}
	if plan.Blocked {
		t.Fatal("plan should not be blocked")
	}
}

func TestComputePlanBlocksAnEmptyDirectory(t *testing.T) {
	locals := []localUser{{ID: "u-1", Username: "one", Active: true}}
	mappings := []mappingRow{{ID: "m-1", ScimUserID: "s1", McharborUserID: "u-1", ScimUsername: "one"}}
	plan := computePlan(nil, nil, mappings, locals, nil)
	if !plan.Blocked || plan.BlockedReason != ReasonEmptyDirectory {
		t.Fatalf("an empty directory with linked accounts must block, got %+v", plan)
	}
}

func TestComputePlanNeverLinksOneAccountTwice(t *testing.T) {
	locals := []localUser{{ID: "u-1", Username: "sam", Email: "sam@example.com", Active: true}}
	mappings := []mappingRow{{ID: "m-1", ScimUserID: "s1", McharborUserID: "u-1", ScimUsername: "sam", ScimEmail: "sam@example.com"}}
	remote := []User{
		remoteUser("s1", "sam", "sam@example.com", true),
		remoteUser("s2", "samuel", "sam@example.com", true),
	}
	plan := computePlan(remote, nil, mappings, locals, nil)
	got := changesByScimID(plan)["s2"]
	if got.Action != ActionSkip || got.Reason != ReasonAlreadyLinked {
		t.Fatalf("second remote identity for a linked account must be skipped, got %+v", got)
	}
}
