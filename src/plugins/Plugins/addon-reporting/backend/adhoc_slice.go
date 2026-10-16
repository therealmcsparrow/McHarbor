// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"sort"
	"strings"
)

// What reaches the model, and nothing else.
//
// Two shapes go out: a digest of the analysed Facts, and capped,
// column-projected slices of the raw snapshot. Both are built here so
// there is exactly one place to audit what leaves the box, and one place
// where the denylist is enforced.

const (
	defaultRowCap   = 50
	maxRowCap       = 200
	sliceByteBudget = 60 << 10
)

// factsDigest renders the analysed facts for the model.
//
// Unassessed groups are omitted entirely rather than sent as zeroes: a
// model shown `Critical: 0` for a security group that was never assessed
// will report "no critical vulnerabilities", which is a different claim
// from "we did not look".
//
// AccessFacts (identity/RBAC counts) is deliberately not included here —
// it is not part of the digest this report type sends to the model.
func factsDigest(f Facts) map[string]any {
	d := map[string]any{}

	if f.Containers.Total > 0 {
		d["containers"] = map[string]any{
			"total": f.Containers.Total, "running": f.Containers.Running,
			"unhealthy": f.Containers.Unhealthy, "stacks": f.Containers.Stacks,
		}
	}
	if f.Security.Assessed {
		d["security"] = map[string]any{
			"critical": f.Security.Critical, "high": f.Security.High,
			"coverage_percent": f.Security.CoveragePercent,
		}
	}
	if f.Capacity.Assessed {
		d["capacity"] = map[string]any{
			"cpu_percent": f.Capacity.CPUPercent, "mem_percent": f.Capacity.MemPercent,
			"headroom_percent": f.Capacity.HeadroomPercent,
			"mem_total_bytes":  f.Capacity.MemTotalBytes,
		}
	}
	if f.Reliability.Assessed {
		d["reliability"] = map[string]any{
			"crashes": f.Reliability.Crashes, "restarts": f.Reliability.Restarts,
			"mean_recovery_minutes": f.Reliability.MeanRecoveryMinutes,
			"recovery_samples":      f.Reliability.RecoverySamples,
			"crash_looping":         len(f.Reliability.CrashLooping),
		}
	}
	if f.Delivery.Assessed {
		d["delivery"] = map[string]any{
			"deployments":         f.Delivery.Deployments,
			"per_week":            f.Delivery.DeploymentsPerWeek,
			"change_failure_rate": f.Delivery.ChangeFailureRate,
		}
	}
	if f.Resilience.Assessed {
		d["resilience"] = map[string]any{
			"runs": f.Resilience.Runs, "success_rate": f.Resilience.SuccessRate,
			"coverage_percent":      f.Resilience.CoveragePercent,
			"last_success_age_days": f.Resilience.LastSuccessAge,
		}
	}
	if f.Availability.Assessed {
		d["availability"] = map[string]any{
			"uptime_percent":       f.Availability.UptimePercent,
			"worst_uptime_percent": f.Availability.WorstUptimePercent,
			"observed_days":        f.Availability.ObservedDays,
			"partial":              f.Availability.Partial,
			"tracked":              f.Availability.Tracked,
		}
	}
	if f.Images.ReclaimableBytes > 0 {
		d["images"] = map[string]any{"reclaimable_bytes": f.Images.ReclaimableBytes}
	}
	return d
}

// sourceDescriptor is one line of the menu the planner chooses from.
type sourceDescriptor struct {
	Key     string   `json:"key"`
	Desc    string   `json:"desc"`
	Columns []string `json:"columns"`
}

// sourceCatalog is the planner's menu. Descriptions are written for a
// model choosing between them, not for a user: they say what question
// the source can answer, because that is what the planner is matching
// the prompt against.
//
// Two invariants, both load-bearing for the planner:
//
//  1. Every entry here must have a projectSource case that returns real
//     data. A source the planner can select but that projects to nil (or
//     an empty slice) makes the report silently under-answer a question
//     it looks like it covered — the exact failure this design forbids.
//     srcSettings is deliberately absent for this reason: collect.go
//     never stores its response on the Snapshot.
//  2. Each entry's Columns must name exactly the map keys its
//     projectSource case emits — no more, no fewer — since the planner
//     selects columns, not just sources, on the promise made here.
//
// The facet entries are merged at runtime: one per (facet, platform) pair
// the installed platform plugins declare, plus Docker's own, so the menu
// offers exactly the platforms this installation can answer for.
func sourceCatalog() []sourceDescriptor {
	return append(builtinSourceCatalog(), facetSourceCatalog()...)
}

func builtinSourceCatalog() []sourceDescriptor {
	return []sourceDescriptor{
		{srcEnvironments, "Configured environments and their connection kind", []string{"name", "kind", "status"}},
		{srcDockerInfo, "Docker daemon version, driver and totals", nil},
		{srcContainers, "Every container: state, status, image, stack", []string{"name", "state", "status", "image", "stack", "created"}},
		{srcImages, "Images: repo, tag, size, whether in use", []string{"repo", "tag", "size_bytes", "in_use"}},
		{srcVolumes, "Volumes: driver, size, whether attached", []string{"name", "driver", "size_bytes", "in_use"}},
		{srcNetworks, "Networks: driver, scope, attached container count", []string{"name", "driver", "scope", "containers"}},
		{srcStacks, "Compose stacks and their service counts", []string{"name", "services", "status"}},
		{srcScans, "Vulnerability scan severity summary per image (counts, not individual CVE findings)", []string{"image", "severity", "scanned_at"}},
		{srcAudit, "Audit log: who did what to which object", []string{"actor", "action", "object", "result", "at"}},
		{srcCompliancePacks, "Compliance pack check verdicts per control framework (evidence for a framework, not a compliance verdict)", []string{"framework", "control", "title", "coverage", "verdict", "reasons"}},
		{srcComplianceAttestations, "Compliance sign-offs: who reviewed and accepted which framework's evidence (including the access review), on which run, when, with what note. Newest first; superseded sign-offs included", []string{"framework", "run", "scope", "attested_by", "attested_at", "note"}},
		{srcRbRunbooks, "Runbooks: the written procedures this instance holds, what each is attached to, how many steps it has, whether it is enabled and whether it runs unattended", []string{"runbook", "attached_to", "steps", "enabled", "unattended", "author", "updated_at"}},
		{srcRunbookRuns, "Runbook runs: who responded to an incident, which runbook they followed, how it ended and which step it stopped at", []string{"runbook", "subject", "responder", "outcome", "stopped_at_step", "stopped_at_title", "incident", "started_at", "finished_at"}},
		{srcActivity, "Container activity events", []string{"container", "action", "at"}},
		{srcAlerts, "Alert rules: target, severity, whether enabled", []string{"name", "target", "severity", "enabled"}},
		{srcEvents, "Alert events: each time an alert rule fired or resolved inside the period, with its severity and what it fired on", []string{"alert", "severity", "state", "subject", "environment", "value", "at"}},
		{srcAutoheal, "Auto-heal restarts: each time the platform restarted an unhealthy container on its own inside the period", []string{"container", "at"}},
		{srcMetrics, "Host-level CPU and memory utilisation (a single aggregate, not per-container samples)", nil},
		{srcUsers, "User accounts and last sign-in", []string{"username", "role", "enabled", "last_login"}},
		{srcRoles, "Roles and their permission counts", []string{"name", "permissions"}},
		{srcGroups, "Groups and their membership counts", []string{"name", "members"}},
		{srcAPIKeys, "API keys: owner, scope, age, last use", []string{"label", "owner", "scopes", "age_days", "last_used"}},
		{srcIdentity, "External identity providers and their status", []string{"provider", "kind", "enabled"}},
		{srcBackups, "Backup plans: target, schedule, retention", []string{"name", "target", "schedule", "retention", "enabled"}},
		{srcBackupLogs, "Backup log entries: action, phase, severity", []string{"plan", "action", "phase", "severity", "at"}},
		{srcStorage, "Configured storage destinations", []string{"name", "kind", "status"}},
		{srcSchedules, "Scheduled jobs and their last run time", []string{"name", "cron", "last_run"}},
		{srcRegistries, "Container registries in use", []string{"name", "url"}},
		{srcUpdates, "Update policies and their last run", []string{"target", "policy", "last_run", "status"}},
		{srcGitops, "GitOps promotion history: environment, state, timestamp", []string{"pipeline_id", "environment", "state", "at"}},
		{srcSystem, "Platform version info (no separate host-system data is collected for this source)", nil},
		{srcVersions, "McHarbor and component versions", nil},
		{srcAgents, "Remote agents: version, connection state, last seen", []string{"name", "version", "state", "last_seen"}},
		{srcPlugins, "Installed plugins and their enabled state", []string{"slug", "version", "enabled"}},
		{srcUptime, "Per-container uptime history over the period", []string{"container", "uptime_percent", "downtime_minutes", "observed_days"}},
		{srcCapacity, "Capacity headroom per filesystem and storage destination: used of total, and how many days until it is full at the current rate (or why no date could be given)", []string{"subject", "kind", "used_percent", "used_bytes", "total_bytes", "days_until_full", "full_on", "percent_per_day", "confidence", "observed_days", "refusal"}},
	}
}

// Sources whose row structs carry credentials, or whose catalog entry
// promises something the collector does not actually decode. Their cases
// emit an explicit short allowlist and are reviewed as security-sensitive.
// See .claude/rules/docker-client.md and kubernetes-client.md.
//
//   - srcEnvironments: envRow never carries the socket path, TCP host,
//     kubeconfig, bearer token or CA cert in the first place — those are
//     dropped at the JSON decode boundary in collect.go. Emits name, kind
//     (connection type), status (IsActive) only.
//   - srcAgents: agentRow carries no token field. Emits name, version,
//     state, last_seen only.
//   - srcRegistries: registryRow never decodes the credential ("Credentials
//     are never decoded" per collect.go). Emits name, url only — see
//     projectSource for why "authenticated" is omitted rather than guessed.
//   - srcSettings: the collector fetches /api/settings/retention into a nil
//     target (collect.go, srcSettings job) — nothing is ever stored on
//     Snapshot for this key, so there is nothing to leak. Returns nil, and
//     is deliberately absent from sourceCatalog (see its doc comment).

// projectSource turns one collected source into what the model may see.
//
// Every projection is an explicit allowlist of fields. That direction
// matters: a denylist would leak the next field someone adds to a row
// struct, and these rows leave the box.
func projectSource(snap *Snapshot, key string, cap int) any {
	if cap <= 0 || cap > maxRowCap {
		cap = defaultRowCap
	}
	switch key {

	case srcEnvironments:
		// Security-sensitive: see the block comment above. envRow itself
		// carries no socket/kubeconfig/token/CA material to begin with.
		out := make([]map[string]any, 0, cap)
		for i, e := range snap.Environments {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": e.Name, "kind": e.Connection, "status": e.IsActive,
			})
		}
		return out

	case srcDockerInfo:
		return map[string]any{
			"version": snap.Docker.ServerVersion, "api_version": snap.Docker.APIVersion,
			"os": snap.Docker.OS, "architecture": snap.Docker.Architecture,
			"kernel_version": snap.Docker.KernelVersion, "driver": snap.Docker.StorageDriver,
			"ncpu": snap.Docker.NCPU, "mem_total_bytes": snap.Docker.MemTotal,
		}

	case srcContainers:
		out := make([]map[string]any, 0, cap)
		for i, c := range snap.Containers {
			if i >= cap {
				break
			}
			name := c.StackService
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			// containerRow has no Health or Restarts field (the Docker
			// list API's narrow decode doesn't carry either); "status"
			// (which often embeds the health parenthetical, e.g. "Up 2
			// hours (healthy)") substitutes for "health", and "restarts"
			// is omitted rather than fabricated.
			out = append(out, map[string]any{
				"name": name, "state": c.State, "status": c.Status,
				"image": c.Image, "stack": c.StackName, "created": c.Created,
			})
		}
		return out

	case srcImages:
		out := make([]map[string]any, 0, cap)
		for i, im := range snap.Images {
			if i >= cap {
				break
			}
			// imageRow has no separate Repo/Tag fields; RepoTags entries
			// come back as a single "repo:tag" string from the Docker
			// API, so split it rather than leaving tag empty. No age_days
			// field exists either (imageRow has no age-related field), so
			// that catalog column was dropped — see sourceCatalog.
			repo, tag := "", ""
			if len(im.RepoTags) > 0 {
				repo, tag = splitRepoTag(im.RepoTags[0])
			}
			out = append(out, map[string]any{
				"repo": repo, "tag": tag,
				"size_bytes": im.Size, "in_use": im.Containers > 0,
			})
		}
		return out

	case srcVolumes:
		out := make([]map[string]any, 0, cap)
		for i, v := range snap.Volumes {
			if i >= cap {
				break
			}
			var sizeBytes int64
			if v.UsageData != nil {
				sizeBytes = v.UsageData.Size
			}
			out = append(out, map[string]any{
				"name": v.Name, "driver": v.Driver,
				"size_bytes": sizeBytes, "in_use": v.RefCount > 0,
			})
		}
		return out

	case srcNetworks:
		out := make([]map[string]any, 0, cap)
		for i, n := range snap.Networks {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": n.Name, "driver": n.Driver, "scope": n.Scope, "containers": n.Containers,
			})
		}
		return out

	case srcStacks:
		out := make([]map[string]any, 0, cap)
		for i, s := range snap.Stacks {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": s.Name, "services": s.Services, "status": s.Status,
			})
		}
		return out

	case srcScans:
		// scanRow is a per-scan summary (severity + Critical/High/Medium/
		// Low counts), not a per-vulnerability row: it has no CVE or
		// fixed_in field, so those two catalog columns are omitted.
		out := make([]map[string]any, 0, cap)
		for i, s := range snap.Scans {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"image": s.ImageRef, "severity": s.Severity, "scanned_at": s.CompletedAt,
			})
		}
		return out

	case srcRbRunbooks:
		out := make([]map[string]any, 0, cap)
		for i, rb := range snap.Runbooks.Runbooks {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"runbook": rb.Name, "attached_to": rbAttachLabel(rb), "steps": len(rb.Steps),
				"enabled": rb.Enabled, "unattended": rb.Automation.active(),
				"author": rb.CreatedBy, "updated_at": rb.UpdatedAt,
			})
		}
		return out

	case srcRunbookRuns:
		out := make([]map[string]any, 0, cap)
		for i, r := range snap.RunbookRuns {
			if i >= cap {
				break
			}
			stopPos, stopTitle := 0, ""
			for _, res := range r.Results {
				if res.StepPosition >= stopPos {
					stopPos, stopTitle = res.StepPosition, res.StepTitle
				}
			}
			out = append(out, map[string]any{
				"runbook": r.RunbookName, "subject": r.SubjectName, "responder": r.StartedBy,
				"outcome": r.Outcome, "stopped_at_step": stopPos, "stopped_at_title": stopTitle,
				"incident": r.IncidentID, "started_at": r.StartedAt, "finished_at": r.FinishedAt,
			})
		}
		return out

	case srcComplianceAttestations:
		out := make([]map[string]any, 0, cap)
		for i, a := range snap.Governance.Packs.Attestations {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"framework": a.Framework, "run": a.RunID, "scope": a.Scope,
				"attested_by": a.AttestedByName, "attested_at": a.AttestedAt, "note": a.Note,
			})
		}
		return out

	case srcCompliancePacks:
		out := make([]map[string]any, 0, cap)
		for _, p := range snap.Governance.Packs.Packs {
			var results map[string]packResultRow
			if p.LatestRun != nil {
				results = map[string]packResultRow{}
				for _, r := range snap.Governance.Packs.Results[p.LatestRun.ID] {
					results[r.ID] = r
				}
			}
			for _, c := range p.Framework.Controls {
				if len(out) >= cap {
					break
				}
				verdict := "not run"
				reasons := ""
				if r, ok := results[c.ID]; ok {
					verdict = r.Status
					reasons = strings.Join(r.Reasons, " ")
				}
				out = append(out, map[string]any{
					"framework": p.Framework.Key, "control": c.ID, "title": c.Title,
					"coverage": c.Coverage, "verdict": verdict, "reasons": reasons,
				})
			}
		}
		return out

	case srcAudit:
		out := make([]map[string]any, 0, cap)
		for i, a := range snap.Audit {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"actor": derefOr(a.Username, ""), "action": a.Action,
				"object": derefOr(a.EntityName, derefOr(a.EntityType, "")),
				"result": a.Severity, "at": a.Timestamp,
			})
		}
		return out

	case srcActivity:
		out := make([]map[string]any, 0, cap)
		for i, a := range snap.Activity {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"container": a.ContainerName, "action": a.Action, "at": a.Timestamp,
			})
		}
		return out

	case srcAlerts:
		// alertRow has no "state" or "last_fired" field: alert firing
		// history is not part of the snapshot. Substituted the closest
		// real fields (severity, enabled) instead of inventing values.
		out := make([]map[string]any, 0, cap)
		for i, al := range snap.Alerts {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": al.Name, "target": al.Target,
				"severity": al.Severity, "enabled": al.Enabled,
			})
		}
		return out

	case srcEvents:
		out := make([]map[string]any, 0, cap)
		for i, e := range snap.AlertEvents {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"alert": e.AlertName, "severity": e.Severity, "state": e.State,
				"subject": e.SubjectName, "environment": e.EnvironmentName,
				"value": e.ValueLabel, "at": e.CreatedAt,
			})
		}
		return out

	case srcAutoheal:
		out := make([]map[string]any, 0, cap)
		for i, a := range snap.AutohealRestarts {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{"container": a.EntityName, "at": a.Timestamp})
		}
		return out

	case srcMetrics:
		// The collector wires srcMetrics to a single host-level aggregate
		// (snap.Host), not a per-container series — Snapshot carries no
		// per-container metric samples. Returned as a scalar object;
		// sourceCatalog's Columns is nil for this source to match.
		return map[string]any{
			"cpu_percent": snap.Host.CPUPercent, "mem_percent": snap.Host.MemPercent,
			"mem_used_bytes": snap.Host.MemUsed, "mem_total_bytes": snap.Host.MemTotal,
			"load1": snap.Host.Load1, "ncpu": snap.Host.NCPU,
		}

	case srcUsers:
		out := make([]map[string]any, 0, cap)
		for i, u := range snap.Users {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"username": u.Username, "role": u.Role,
				"enabled": u.IsActive, "last_login": derefOr(u.LastLogin, ""),
			})
		}
		return out

	case srcRoles:
		// roleRow has no per-role user count; "users" is omitted rather
		// than fabricated. "permissions" is the count of permission
		// strings, matching the catalog's "permission counts" wording.
		out := make([]map[string]any, 0, cap)
		for i, r := range snap.Roles {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": r.Name, "permissions": len(r.Permissions),
			})
		}
		return out

	case srcGroups:
		// groupRow has no per-group role count; "roles" is omitted.
		out := make([]map[string]any, 0, cap)
		for i, g := range snap.Groups {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": g.Name, "members": g.MemberCount,
			})
		}
		return out

	case srcAPIKeys:
		out := make([]map[string]any, 0, cap)
		for i, k := range snap.APIKeys {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"label": k.Name, "owner": k.Username, "scopes": k.Scopes,
				"age_days": int(daysSince(k.CreatedAt)), "last_used": derefOr(k.LastUsedAt, ""),
			})
		}
		return out

	case srcIdentity:
		// identityRow has no per-provider user count; "users" is omitted.
		out := make([]map[string]any, 0, cap)
		for i, id := range snap.Identity {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"provider": id.Name, "kind": id.Type, "enabled": id.Enabled,
			})
		}
		return out

	case srcBackups:
		out := make([]map[string]any, 0, cap)
		for i, b := range snap.BackupPlans {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": b.Name, "target": b.ContainerName, "schedule": b.Cron,
				"retention": b.RetentionDays, "enabled": b.Enabled,
			})
		}
		return out

	case srcBackupLogs:
		// The srcBackupLogs collector job populates snap.BackupLogs (a
		// phase/message stream), not snap.BackupRuns (which is fetched
		// alongside srcBackups instead). backupLogRow carries no
		// size_bytes or duration_s, so those two catalog columns are
		// omitted rather than guessed; phase/severity substitute for
		// "status".
		out := make([]map[string]any, 0, cap)
		for i, l := range snap.BackupLogs {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"plan": derefOr(l.PlanName, ""), "action": l.Action,
				"phase": l.Phase, "severity": l.Severity, "at": l.CreatedAt,
			})
		}
		return out

	case srcStorage:
		out := make([]map[string]any, 0, cap)
		for i, s := range snap.Storage {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": s.Name, "kind": s.Type, "status": s.Enabled,
			})
		}
		return out

	case srcSchedules:
		// scheduleRow has no last-run status field; "last_status" is
		// omitted rather than guessed.
		out := make([]map[string]any, 0, cap)
		for i, s := range snap.Schedules {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": s.Name, "cron": s.Cron, "last_run": s.LastRunAt,
			})
		}
		return out

	case srcRegistries:
		// Security-sensitive: see the block comment above. registryRow
		// never decodes the credential, so there is no real field to
		// report an "authenticated" bool from; fabricating one would be
		// worse than omitting it, so only name and url are emitted.
		out := make([]map[string]any, 0, cap)
		for i, r := range snap.Registries {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": r.Name, "url": r.URL,
			})
		}
		return out

	case srcUpdates:
		out := make([]map[string]any, 0, cap)
		for i, u := range snap.Updates {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"target": u.ContainerName, "policy": u.Name,
				"last_run": u.LastRunAt, "status": u.LastRunStatus,
			})
		}
		return out

	case srcGitops:
		// pipelineRow (Name, Repo, Enabled) has no environment/state/at
		// fields, so the promotions slice is projected instead — it
		// actually carries environment (ToEnv), state (Status) and at
		// (CreatedAt). Emits "pipeline_id", not "name" (promotionRow has
		// no Name field) — sourceCatalog's Columns matches this.
		out := make([]map[string]any, 0, cap)
		for i, p := range snap.Promotions {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"pipeline_id": p.PipelineID, "environment": p.ToEnv,
				"state": p.Status, "at": p.CreatedAt,
			})
		}
		return out

	case srcSystem:
		// The srcSystem collector job fetches /api/versions into
		// snap.Versions (same target as srcVersions) rather than any
		// host-system struct, despite the catalog description — Snapshot
		// has no separate "system" field. Projected from what is
		// actually populated.
		return map[string]any{
			"mcharbor_version": snap.Versions.McHarbor.Version,
			"mcharbor_commit":  snap.Versions.McHarbor.Commit,
			"mcharbor_built":   snap.Versions.McHarbor.Built,
		}

	case srcVersions:
		agents := make([]map[string]any, 0, len(snap.Versions.Agents))
		for _, a := range snap.Versions.Agents {
			agents = append(agents, map[string]any{"name": a.Name, "version": a.Version})
		}
		return map[string]any{
			"mcharbor_version": snap.Versions.McHarbor.Version,
			"mcharbor_commit":  snap.Versions.McHarbor.Commit,
			"mcharbor_built":   snap.Versions.McHarbor.Built,
			"agents":           agents,
		}

	case srcAgents:
		// Security-sensitive: see the block comment above. agentRow
		// carries no token field; still allowlisted explicitly.
		out := make([]map[string]any, 0, cap)
		for i, a := range snap.Agents {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"name": a.Name, "version": a.Version, "state": a.Status, "last_seen": a.LastSeen,
			})
		}
		return out

	case srcPlugins:
		out := make([]map[string]any, 0, cap)
		for i, p := range snap.Plugins {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"slug": p.Slug, "version": p.Version, "enabled": p.Enabled,
			})
		}
		return out

	case srcSettings:
		// Security-sensitive: see the block comment above. The collector
		// discards the settings response at the source (fetched into a
		// nil target), so Snapshot never carries settings data at all.
		// Deliberately absent from sourceCatalog — do not re-add it there
		// without also storing something on Snapshot for it to project.
		return nil

	case srcCapacity:
		out := make([]map[string]any, 0, cap)
		for i, c := range snap.Capacity.Subjects {
			if i >= cap {
				break
			}
			// A refused subject is printed with its reason rather than dropped:
			// "insufficient history" is an answer an operator can act on and a
			// blank page is not.
			row := map[string]any{
				"subject": c.SubjectName, "kind": c.SubjectKind,
				"used_percent": c.UsedPercent, "used_bytes": c.UsedBytes, "total_bytes": c.TotalBytes,
				"percent_per_day": c.PercentPerDay, "confidence": c.R2,
				"observed_days": c.ObservedDays, "refusal": c.Refusal,
				"days_until_full": nil, "full_on": nil,
			}
			if c.Red != nil {
				row["days_until_full"] = c.Red.Days
				row["full_on"] = c.Red.At
			}
			out = append(out, row)
		}
		return out

	case srcUptime:
		out := make([]map[string]any, 0, cap)
		for i, u := range snap.Uptime.Containers {
			if i >= cap {
				break
			}
			out = append(out, map[string]any{
				"container": u.ContainerName, "uptime_percent": u.UptimePercent,
				"downtime_minutes": u.DowntimeMinutes, "observed_days": u.Days,
			})
		}
		return out
	}
	if facet, platform, ok := parseFacetSource(key); ok {
		return projectFacet(snap, facet, platform, cap)
	}
	return nil
}

// splitRepoTag splits a Docker "repo:tag" string, as returned in
// imageRow.RepoTags, into its repo and tag parts. The split happens on
// the last colon after the last slash, so a registry host:port
// (e.g. "registry.example.com:5000/app:1.0") is not mistaken for a tag
// separator.
func splitRepoTag(s string) (repo, tag string) {
	slash := strings.LastIndex(s, "/")
	colon := strings.LastIndex(s, ":")
	if colon > slash {
		return s[:colon], s[colon+1:]
	}
	return s, ""
}

// buildSlices projects every planned source and trims to budget.
//
// Trimming is round-robin rather than in plan order: a single large
// source (audit, events) would otherwise consume the whole budget and
// starve the sources that actually answer the question.
func buildSlices(snap *Snapshot, plan AdHocPlan, budget int) map[string]any {
	if budget <= 0 {
		budget = sliceByteBudget
	}
	caps := make(map[string]int, len(plan.Sources))
	for _, k := range plan.Sources {
		c := plan.Caps[k]
		if c <= 0 || c > maxRowCap {
			c = defaultRowCap
		}
		caps[k] = c
	}

	keys := append([]string(nil), plan.Sources...)
	sort.Strings(keys)

	for {
		out := make(map[string]any, len(keys))
		for _, k := range keys {
			if v := projectSource(snap, k, caps[k]); v != nil {
				out[k] = v
			}
		}
		raw, err := json.Marshal(out)
		if err != nil || len(raw) <= budget {
			return out
		}
		// Halve every row cap and try again. Terminates because caps
		// reach 1 and then 0, at which point every projection is empty
		// and the payload is under any positive budget.
		shrunk := false
		for _, k := range keys {
			if caps[k] > 1 {
				caps[k] /= 2
				shrunk = true
			}
		}
		if !shrunk {
			return out
		}
	}
}
