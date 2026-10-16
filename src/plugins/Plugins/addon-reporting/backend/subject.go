// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"context"
	"strings"
)

// Report scope: narrowing a report to one object.
//
// Several report types answer a question about a *thing* — how did this
// workload behave, what does this user do, what is this group allowed to
// touch — and reading the estate-wide version of that report to find one
// row is a poor substitute. A subject is therefore a filter applied to
// the collected snapshot, once, before analysis: every builder then sees
// an estate that contains only the subject and what belongs to it, and
// none of them needs to know the filter exists.
//
// Filtering happens after collection rather than during it because the
// host's list endpoints do not all accept an object filter, and asking
// each one differently would put a second, weaker copy of this logic in
// the collector. The cost is fetching a little more than is used; the
// benefit is one place that decides what "belongs to" means.

// SubjectKind is the kind of object a report can be narrowed to.
type SubjectKind string

const (
	SubjectContainer SubjectKind = "container"
	SubjectStack     SubjectKind = "stack"
	SubjectImage     SubjectKind = "image"
	SubjectVolume    SubjectKind = "volume"
	SubjectNetwork   SubjectKind = "network"
	SubjectUser      SubjectKind = "user"
	SubjectGroup     SubjectKind = "group"
	SubjectRunbook   SubjectKind = "runbook"
)

// Subject is the chosen narrowing. An empty Kind means the whole estate.
type Subject struct {
	Kind SubjectKind `json:"kind,omitempty"`
	ID   string      `json:"id,omitempty"`
	Name string      `json:"name,omitempty"`
}

func (s Subject) empty() bool {
	return s.Kind == "" || (strings.TrimSpace(s.ID) == "" && strings.TrimSpace(s.Name) == "")
}

// label is what the cover prints.
func (s Subject) label() string {
	if name := strings.TrimSpace(s.Name); name != "" {
		return name
	}
	return strings.TrimSpace(s.ID)
}

// parseSubjectKind accepts only kinds this plugin knows, so a typo in a
// request narrows nothing rather than silently emptying a report.
func parseSubjectKind(raw string) (SubjectKind, bool) {
	switch SubjectKind(strings.TrimSpace(strings.ToLower(raw))) {
	case SubjectContainer:
		return SubjectContainer, true
	case SubjectStack:
		return SubjectStack, true
	case SubjectImage:
		return SubjectImage, true
	case SubjectVolume:
		return SubjectVolume, true
	case SubjectNetwork:
		return SubjectNetwork, true
	case SubjectUser:
		return SubjectUser, true
	case SubjectGroup:
		return SubjectGroup, true
	case SubjectRunbook:
		return SubjectRunbook, true
	}
	return "", false
}

// SubjectOption is one row in the studio's scope picker.
type SubjectOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Hint is secondary text — a stack name, a state, an email — shown
	// next to the name so two similarly named objects can be told apart.
	Hint string `json:"hint,omitempty"`
}

// listSubjects fetches the objects of one kind the calling user may see.
//
// It goes through the same loopback client the collector uses, so the
// picker can never offer an object the user could not already open in
// the interface.
func listSubjects(ctx context.Context, api *hostAPI, kind SubjectKind) []SubjectOption {
	out := []SubjectOption{}
	switch kind {
	case SubjectContainer:
		var rows []containerRow
		api.getList(ctx, "subjects", "Containers", "/api/containers",
			map[string]string{"all": "true", "per_page": "500"}, &rows)
		for _, c := range rows {
			out = append(out, SubjectOption{
				ID: c.ID, Name: containerName(c), Hint: orDash(c.StackName, c.State),
			})
		}
	case SubjectStack:
		var rows []stackRow
		api.getList(ctx, "subjects", "Stacks", "/api/stacks", nil, &rows)
		for _, s := range rows {
			out = append(out, SubjectOption{ID: s.Name, Name: s.Name, Hint: s.Status})
		}
	case SubjectImage:
		var rows []imageRow
		api.getList(ctx, "subjects", "Images", "/api/images", map[string]string{"per_page": "500"}, &rows)
		for _, im := range rows {
			out = append(out, SubjectOption{ID: im.ID, Name: imageName(im), Hint: shortID(im.ID)})
		}
	case SubjectVolume:
		var rows []volumeRow
		api.getList(ctx, "subjects", "Volumes", "/api/volumes", map[string]string{"per_page": "500"}, &rows)
		for _, v := range rows {
			out = append(out, SubjectOption{ID: v.Name, Name: v.Name, Hint: v.Driver})
		}
	case SubjectNetwork:
		var rows []networkRow
		api.getList(ctx, "subjects", "Networks", "/api/networks", nil, &rows)
		for _, n := range rows {
			out = append(out, SubjectOption{ID: n.ID, Name: n.Name, Hint: n.Driver})
		}
	case SubjectUser:
		var rows []userRow
		api.getList(ctx, "subjects", "Users", "/api/users", map[string]string{"per_page": "500"}, &rows)
		for _, u := range rows {
			hint := u.Role
			if u.Email != nil && *u.Email != "" {
				hint = *u.Email
			}
			out = append(out, SubjectOption{ID: u.ID, Name: u.Username, Hint: hint})
		}
	case SubjectGroup:
		var rows []groupRow
		api.getList(ctx, "subjects", "Groups", "/api/groups", nil, &rows)
		for _, g := range rows {
			out = append(out, SubjectOption{ID: g.ID, Name: g.Name, Hint: g.Description})
		}
	case SubjectRunbook:
		var rows []rbRunbookRow
		api.getList(ctx, "subjects", "Runbooks", "/api/runbooks",
			map[string]string{"per_page": "500"}, &rows)
		for _, rb := range rows {
			out = append(out, SubjectOption{ID: rb.ID, Name: rb.Name, Hint: rbAttachLabel(rb)})
		}
	}
	return out
}

// applySubject narrows a collected snapshot in place.
//
// Nothing is invented: a collection is only filtered when the wire shape
// carries an unambiguous link back to the subject. Where no such link
// exists the collection is left whole rather than emptied, because an
// emptied list reads to a builder as "there are none", which would be a
// lie told with a straight face.
func applySubject(ctx context.Context, api *hostAPI, snap *Snapshot, sub Subject) {
	if snap == nil || sub.empty() {
		return
	}
	snap.Subject = sub

	switch sub.Kind {
	case SubjectContainer:
		narrowToContainers(snap, pickContainers(snap, func(c containerRow) bool {
			return matchID(c.ID, sub.ID) || strings.EqualFold(containerName(c), sub.Name)
		}))
	case SubjectStack:
		name := subjectKey(sub)
		narrowToContainers(snap, pickContainers(snap, func(c containerRow) bool {
			return strings.EqualFold(c.StackName, name)
		}))
		snap.Stacks = filterStacks(snap.Stacks, name)
	case SubjectImage:
		keepImages := filterImages(snap.Images, sub)
		narrowToContainers(snap, pickContainers(snap, func(c containerRow) bool {
			return matchID(c.ImageID, sub.ID) || strings.EqualFold(c.Image, sub.Name)
		}))
		snap.Images = keepImages
	case SubjectVolume:
		name := subjectKey(sub)
		keepVolumes := filterVolumes(snap.Volumes, name)
		narrowToContainers(snap, pickContainers(snap, func(c containerRow) bool {
			return containerUsesVolume(c, name)
		}))
		snap.Volumes = keepVolumes
	case SubjectNetwork:
		keepNetworks := filterNetworks(snap.Networks, sub)
		narrowToContainers(snap, pickContainers(snap, func(c containerRow) bool {
			return containerUsesNetwork(c, sub)
		}))
		snap.Networks = keepNetworks
	case SubjectUser:
		narrowToUsers(snap, pickUsers(snap, func(u userRow) bool {
			return u.ID == sub.ID || strings.EqualFold(u.Username, sub.Name)
		}))
	case SubjectGroup:
		narrowToGroup(ctx, api, snap, sub)
	case SubjectRunbook:
		narrowToRunbook(snap, sub)
	}
}

// subjectKey prefers the name for the kinds whose identity *is* the name
// (stacks and volumes), falling back to the id the picker sent.
func subjectKey(sub Subject) string {
	if name := strings.TrimSpace(sub.Name); name != "" {
		return name
	}
	return strings.TrimSpace(sub.ID)
}

// matchID compares two container or image ids that may be abbreviated on
// either side — the picker sends a full id, but a report may hold a
// twelve-character short form.
func matchID(full, want string) bool {
	full = strings.TrimPrefix(strings.TrimSpace(full), "sha256:")
	want = strings.TrimPrefix(strings.TrimSpace(want), "sha256:")
	if full == "" || want == "" {
		return false
	}
	return strings.HasPrefix(full, want) || strings.HasPrefix(want, full)
}

func pickContainers(snap *Snapshot, keep func(containerRow) bool) []containerRow {
	out := make([]containerRow, 0, 1)
	for _, c := range snap.Containers {
		if keep(c) {
			out = append(out, c)
		}
	}
	return out
}

func pickUsers(snap *Snapshot, keep func(userRow) bool) []userRow {
	out := make([]userRow, 0, 1)
	for _, u := range snap.Users {
		if keep(u) {
			out = append(out, u)
		}
	}
	return out
}

func containerUsesVolume(c containerRow, name string) bool {
	for _, m := range c.Mounts {
		if strings.EqualFold(m.Type, "volume") && strings.EqualFold(m.Name, name) {
			return true
		}
	}
	return false
}

func containerUsesNetwork(c containerRow, sub Subject) bool {
	for name, n := range c.NetworkSettings.Networks {
		if strings.EqualFold(name, sub.Name) || matchID(n.NetworkID, sub.ID) {
			return true
		}
	}
	return false
}

// narrowToContainers is the workload cascade: everything that hangs off a
// container follows the container set.
func narrowToContainers(snap *Snapshot, keep []containerRow) {
	names := map[string]bool{}
	ids := map[string]bool{}
	images := map[string]bool{}
	stacks := map[string]bool{}
	volumes := map[string]bool{}
	networks := map[string]bool{}

	for _, c := range keep {
		names[strings.ToLower(containerName(c))] = true
		ids[strings.ToLower(strings.TrimSpace(c.ID))] = true
		if c.Image != "" {
			images[strings.ToLower(c.Image)] = true
		}
		if c.ImageID != "" {
			images[strings.ToLower(strings.TrimPrefix(c.ImageID, "sha256:"))] = true
		}
		if c.StackName != "" {
			stacks[strings.ToLower(c.StackName)] = true
		}
		for _, m := range c.Mounts {
			if strings.EqualFold(m.Type, "volume") && m.Name != "" {
				volumes[strings.ToLower(m.Name)] = true
			}
		}
		for name := range c.NetworkSettings.Networks {
			networks[strings.ToLower(name)] = true
		}
	}

	snap.Containers = keep
	snap.Dashboard = dashboardFor(snap, keep)

	snap.Stacks = keepIf(snap.Stacks, func(s stackRow) bool { return stacks[strings.ToLower(s.Name)] })
	snap.Images = keepIf(snap.Images, func(im imageRow) bool {
		if images[strings.ToLower(strings.TrimPrefix(im.ID, "sha256:"))] {
			return true
		}
		for _, tag := range im.RepoTags {
			if images[strings.ToLower(tag)] {
				return true
			}
		}
		return false
	})
	snap.Volumes = keepIf(snap.Volumes, func(v volumeRow) bool { return volumes[strings.ToLower(v.Name)] })
	snap.Networks = keepIf(snap.Networks, func(n networkRow) bool { return networks[strings.ToLower(n.Name)] })

	snap.Scans = keepIf(snap.Scans, func(s scanRow) bool { return images[strings.ToLower(s.ImageRef)] })
	snap.Activity = keepIf(snap.Activity, func(a activityRow) bool {
		return ids[strings.ToLower(a.ContainerID)] || names[strings.ToLower(strings.TrimPrefix(a.ContainerName, "/"))]
	})
	snap.Audit = keepIf(snap.Audit, func(a auditRow) bool {
		return a.EntityName != nil && names[strings.ToLower(*a.EntityName)]
	})
	snap.Alerts = keepIf(snap.Alerts, func(a alertRow) bool {
		t := strings.ToLower(strings.TrimPrefix(a.Target, "/"))
		return names[t] || ids[t] || stacks[t]
	})
	snap.BackupPlans = keepIf(snap.BackupPlans, func(b backupPlanRow) bool {
		return names[strings.ToLower(b.ContainerName)]
	})
	snap.BackupRuns = keepIf(snap.BackupRuns, func(b backupRunRow) bool {
		return names[strings.ToLower(b.ContainerName)]
	})
	snap.BackupLogs = keepIf(snap.BackupLogs, func(b backupLogRow) bool {
		return b.ContainerName != nil && names[strings.ToLower(*b.ContainerName)]
	})
	snap.Updates = keepIf(snap.Updates, func(u updatePolicyRow) bool {
		return names[strings.ToLower(u.ContainerName)]
	})
	snap.UpdateHist = keepIf(snap.UpdateHist, func(u updateHistoryRow) bool {
		return names[strings.ToLower(strings.TrimPrefix(u.Container, "/"))]
	})

	snap.Uptime.Containers = keepIf(snap.Uptime.Containers, func(u uptimeRow) bool {
		return ids[strings.ToLower(u.ContainerID)] || names[strings.ToLower(strings.TrimPrefix(u.ContainerName, "/"))]
	})
	recomputeUptimeTotals(&snap.Uptime)
}

// narrowToUsers is the identity cascade.
//
// Groups are left whole: the group list carries a member count but no
// membership, so there is no honest way to say which groups a user is in
// from what was collected.
func narrowToUsers(snap *Snapshot, keep []userRow) {
	names := map[string]bool{}
	roles := map[string]bool{}
	for _, u := range keep {
		names[strings.ToLower(u.Username)] = true
		if u.Role != "" {
			roles[strings.ToLower(u.Role)] = true
		}
	}
	snap.Users = keep
	snap.Audit = keepIf(snap.Audit, func(a auditRow) bool {
		return a.Username != nil && names[strings.ToLower(*a.Username)]
	})
	snap.APIKeys = keepIf(snap.APIKeys, func(k apiKeyRow) bool { return names[strings.ToLower(k.Username)] })
	if len(roles) > 0 {
		snap.Roles = keepIf(snap.Roles, func(r roleRow) bool { return roles[strings.ToLower(r.Name)] })
	}
}

// narrowToGroup resolves the group's membership before applying the
// identity cascade. The list endpoint carries only a member count, so the
// single group is fetched to learn who is in it.
func narrowToGroup(ctx context.Context, api *hostAPI, snap *Snapshot, sub Subject) {
	snap.Groups = keepIf(snap.Groups, func(g groupRow) bool {
		return g.ID == sub.ID || strings.EqualFold(g.Name, sub.Name)
	})

	var detail struct {
		Members []struct {
			Username string `json:"username"`
		} `json:"members"`
	}
	if id := strings.TrimSpace(sub.ID); id != "" && api != nil {
		api.get(ctx, "subject_group", "Group membership", "/api/groups/"+id, nil, &detail)
	}
	if len(detail.Members) == 0 {
		// Without membership the honest narrowing is the group row alone;
		// filtering users on a guess would misattribute activity.
		return
	}
	members := map[string]bool{}
	for _, m := range detail.Members {
		members[strings.ToLower(m.Username)] = true
	}
	narrowToUsers(snap, pickUsers(snap, func(u userRow) bool { return members[strings.ToLower(u.Username)] }))
}

func filterStacks(rows []stackRow, name string) []stackRow {
	return keepIf(rows, func(s stackRow) bool { return strings.EqualFold(s.Name, name) })
}

func filterVolumes(rows []volumeRow, name string) []volumeRow {
	return keepIf(rows, func(v volumeRow) bool { return strings.EqualFold(v.Name, name) })
}

func filterNetworks(rows []networkRow, sub Subject) []networkRow {
	return keepIf(rows, func(n networkRow) bool {
		return matchID(n.ID, sub.ID) || strings.EqualFold(n.Name, sub.Name)
	})
}

func filterImages(rows []imageRow, sub Subject) []imageRow {
	return keepIf(rows, func(im imageRow) bool {
		if matchID(im.ID, sub.ID) {
			return true
		}
		for _, tag := range im.RepoTags {
			if strings.EqualFold(tag, sub.Name) {
				return true
			}
		}
		return false
	})
}

// keepIf filters a slice, returning an empty non-nil slice rather than
// nil so a builder can still tell "the source answered" from "the source
// was not consulted".
func keepIf[T any](rows []T, keep func(T) bool) []T {
	if rows == nil {
		return nil
	}
	out := make([]T, 0, len(rows))
	for _, r := range rows {
		if keep(r) {
			out = append(out, r)
		}
	}
	return out
}

// dashboardFor rebuilds the estate counters from the narrowed data, so a
// scoped report's headline numbers agree with its tables.
func dashboardFor(snap *Snapshot, keep []containerRow) dashboardStats {
	var d dashboardStats
	for _, c := range keep {
		d.Containers.Total++
		switch strings.ToLower(c.State) {
		case "running":
			d.Containers.Running++
		case "paused":
			d.Containers.Paused++
		case "restarting":
			d.Containers.Restarting++
		case "dead":
			d.Containers.Killed++
		default:
			d.Containers.Stopped++
		}
	}
	d.Images = len(snap.Images)
	d.Volumes = len(snap.Volumes)
	d.Networks = len(snap.Networks)
	d.Stacks = len(snap.Stacks)
	return d
}

// recomputeUptimeTotals rebuilds the fleet roll-up from whatever rows
// survived the filter. The host computed the original totals across the
// whole estate; leaving them would put an estate-wide availability figure
// on a report about one workload.
func recomputeUptimeTotals(u *uptimeSummary) {
	var t uptimeTotalRow
	worst := -1.0
	meanSum, meanN := 0.0, 0

	for _, r := range u.Containers {
		t.Containers++
		if r.Samples > 0 {
			t.Tracked++
			meanSum += r.UptimePercent
			meanN++
			if worst < 0 || r.UptimePercent < worst {
				worst = r.UptimePercent
			}
		}
		if r.DowntimeMinutes > 0 {
			t.WithDowntime++
		}
		if r.HasHealthcheck {
			t.WithHealthcheck++
		}
		t.Samples += r.Samples
		t.ObservedMinutes += r.ObservedMinutes
		t.DowntimeMinutes += r.DowntimeMinutes
		t.Outages += r.Outages
		t.Transitions += r.Transitions
		t.Probes += r.Probes
		t.ProbesFailed += r.ProbesFailed
	}

	if t.ObservedMinutes > 0 {
		t.UptimePercent = (t.ObservedMinutes - t.DowntimeMinutes) / t.ObservedMinutes * 100
	}
	if t.Probes > 0 {
		t.ProbePassPercent = float64(t.Probes-t.ProbesFailed) / float64(t.Probes) * 100
	}
	if meanN > 0 {
		t.MeanUptimePercent = meanSum / float64(meanN)
	}
	if worst >= 0 {
		t.WorstUptimePercent = worst
	}
	u.Totals = t
}
