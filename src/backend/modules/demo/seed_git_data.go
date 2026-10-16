// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package demo

import (
	"time"

	"github.com/therealmcsparrow/mcharbor/modules/gitops"
)

const (
	hour = time.Hour
	day  = 24 * time.Hour
)

// gitCommitSpec is one commit on a demo repository's tracked branch; ago is how
// long before the seed it was pushed.
type gitCommitSpec struct {
	message string
	ago     time.Duration
}

// gitRepoSpec is one demo repository. Its credential is a placeholder that no
// forge would accept, stored through the same encryption the git module uses.
type gitRepoSpec struct {
	key, name, url, branch, path string
	authType, credential         string
	autoSync                     bool
	interval                     int
	env                          []string
	webhook                      bool
	syncAgo                      time.Duration
	syncError                    string
	commits                      []gitCommitSpec
	synced                       []int
	failedSyncs                  []time.Duration
}

// gitStageSpec is one stage; env lists estate host keys in order of preference,
// so a smaller demo estate still resolves every stage to a real environment.
type gitStageSpec struct {
	name     string
	mode     gitops.DeployMode
	env      []string
	approval bool
}

// gitPipelineSpec is one demo pipeline over one of the repositories above.
type gitPipelineSpec struct {
	key, name, repo, description string
	trigger                      gitops.TriggerType
	stages                       []gitStageSpec
	previewDomain, previewOwner  string
}

// gitPromoSpec is one promotion in a pipeline's history. by is a demo username,
// or the system actor the real code records.
type gitPromoSpec struct {
	pipeline, stage string
	commit          int
	status          string
	by, note        string
	ago, took       time.Duration
	lastError       string
}

// gitApprovalSpec is one sign-off request between stages. Who asked and when is
// derived from the promotion into the stage before, which is what raised it.
type gitApprovalSpec struct {
	pipeline, stage    string
	commit             int
	status, resolvedBy string
	note               string
	resolvedAfter      time.Duration
}

// gitPushSpec is one head commit a pull request's preview was deployed at.
type gitPushSpec struct {
	head string
	ago  time.Duration
}

// gitPreviewSpec is one pull-request preview. Only an active one keeps its
// environment; a terminal one looks the way TeardownPreview leaves the row. For a
// failed one, lastError is the failed deploy's own message, which failPreview prefixes.
type gitPreviewSpec struct {
	pipeline, pr, title, source string
	pushes                      []gitPushSpec
	status                      string
	closedAgo                   time.Duration
	tornDownBy, lastError       string
	hostKey                     string
}

// demoPreviewHostKey is the estate host the one active preview runs on.
const demoPreviewHostKey = "preview-storefront-pr-482"

const (
	tokenPlaceholder = "demo-only-placeholder-token-not-valid-anywhere"
	basicPlaceholder = "deploy-bot:demo-only-placeholder-password"
	infraSyncError   = "git: fetching main: authentication required: HTTP Basic: Access denied. " +
		"The provided password or token is incorrect or your account has 2FA enabled and you must use a " +
		"personal access token instead of a password."
)

var demoGitRepos = []gitRepoSpec{
	{
		key: "storefront", name: "storefront", url: "https://github.com/acme-retail/storefront.git",
		branch: "main", path: "deploy/compose/docker-compose.yml", authType: "token", credential: tokenPlaceholder,
		autoSync: true, interval: 300, env: []string{"dev"}, webhook: true, syncAgo: 4 * time.Minute,
		commits: []gitCommitSpec{
			{"feat(search): typo-tolerant product search", 13 * day},
			{"fix(api): return 409 when a cart is checked out twice", 10 * day},
			{"chore(deps): bump node to 22.9 and pnpm to 9.11", 7 * day},
			{"feat(checkout): Apple Pay and Google Pay wallets", 4 * day},
			{"fix(web): lazy-load hero images below the fold", 26 * hour},
			{"perf(api): cache the category tree for 60s", 9 * time.Minute},
		},
		synced: []int{0, 1, 2, 3, 4, 5},
	},
	{
		key: "platform-infra", name: "platform-infra", url: "https://gitlab.acme.example/platform/infra.git",
		branch: "main", path: "stacks/observability/docker-compose.yml", authType: "basic", credential: basicPlaceholder,
		autoSync: true, interval: 600, env: []string{"prod-eu-1"}, webhook: true, syncAgo: 17 * time.Minute,
		syncError: infraSyncError,
		commits: []gitCommitSpec{
			{"observability: Grafana 11.2 and Loki 3.2", 16 * day},
			{"alertmanager: route payments pages to the SRE rota", 9 * day},
			{"prometheus: raise retention to 30d on prod", 5 * day},
			{"otel-collector: sample 10% of healthy traces", 2 * day},
		},
		synced:      []int{0, 1, 2},
		failedSyncs: []time.Duration{47 * hour, 20 * hour, 17 * time.Minute},
	},
	{
		key: "homelab-compose", name: "homelab-compose", url: "https://git.lab.acme-home.example/ops/homelab-compose.git",
		branch: "main", path: "media/docker-compose.yml", authType: "none",
		interval: 3600, env: []string{"homelab"}, syncAgo: 3 * hour,
		commits: []gitCommitSpec{
			{"jellyfin: enable hardware transcoding on the iGPU", 20 * day},
			{"immich: move the library to the new ZFS dataset", 6 * day},
			{"paperless: nightly export to the NAS", 3 * hour},
		},
		synced: []int{0, 1, 2},
	},
	{
		key: "edge-pos", name: "edge-pos", url: "https://github.com/acme-retail/edge-pos.git",
		branch: "release/2026.09", path: "compose.yaml", authType: "token", credential: tokenPlaceholder,
		autoSync: true, interval: 900, env: []string{"edge-store-04"}, webhook: true, syncAgo: 11 * time.Minute,
		commits: []gitCommitSpec{
			{"pos: receipt printer driver 4.2 for the Epson TM-m30III", 12 * day},
			{"pos: offline queue survives a power cut", 8 * day},
			{"pos: EUR rounding for the Paris cash drawers", 3 * day},
			{"cameras: lower the RTSP bitrate on the store uplink", 20 * hour},
		},
		synced: []int{0, 1, 2, 3},
	},
}

var demoPipelines = []gitPipelineSpec{
	{
		key: "storefront", name: "Storefront", repo: "storefront", trigger: gitops.TriggerTypeAuto,
		description: "Web shop: every push to main lands on dev and staging; production needs a sign-off",
		stages: []gitStageSpec{
			{"dev", gitops.DeployAuto, []string{"dev"}, false},
			{"staging", gitops.DeployAuto, []string{"staging-eu", "staging-us"}, false},
			{"production", gitops.DeployManual, []string{"prod-eu-1", "prod-eu-2"}, true},
		},
	},
	{
		key: "storefront-previews", name: "Storefront PR previews", repo: "storefront", trigger: gitops.TriggerTypePRPreview,
		description:   "One throw-away environment per open pull request, gone 72 hours after it opens",
		stages:        []gitStageSpec{{"preview", gitops.DeployPRPreview, nil, false}},
		previewDomain: "preview.acme.example", previewOwner: "a.jansen",
	},
	{
		key: "observability", name: "Observability stack", repo: "platform-infra", trigger: gitops.TriggerTypeManual,
		description: "Prometheus, Grafana, Loki and Alertmanager, promoted by hand",
		stages: []gitStageSpec{
			{"dev", gitops.DeployManual, []string{"dev"}, false},
			{"staging", gitops.DeployManual, []string{"staging-eu", "staging-us"}, false},
			{"prod", gitops.DeployManual, []string{"prod-eu-1", "prod-us-1"}, true},
		},
	},
	{
		key: "edge-pos", name: "Edge POS rollout", repo: "edge-pos", trigger: gitops.TriggerTypeAuto,
		description: "Point-of-sale release train: one pilot store, then two waves, each wave signed off",
		stages: []gitStageSpec{
			{"pilot", gitops.DeployAuto, []string{"edge-store-04", "edge-store-03", "edge-store-02", "edge-store-01"}, false},
			{"wave-1", gitops.DeployManual, []string{"edge-store-01", "edge-store-02", "edge-store-03"}, true},
			{"wave-2", gitops.DeployManual, []string{"edge-store-02", "edge-store-03", "edge-store-01"}, true},
		},
	},
}

const (
	schedulerActor = "gitops-scheduler"
	autoNote       = "Promoted automatically on a new commit"
	afterApproval  = "auto-promoted after approval"
	manifestError  = "deploying stack {stack} failed: docker compose up failed: manifest for " +
		"ghcr.io/acme-retail/storefront-api:sha-{short} not found: manifest unknown: manifest unknown"
	infraFetchError = "fetching the compose file failed: fetching commit {sha}: authentication required"
)

// demoPromotions is each pipeline's history. A promotion that followed an
// approval carries no actor or time of its own: both come from the approval.
var demoPromotions = []gitPromoSpec{
	{"storefront", "dev", 0, "succeeded", schedulerActor, autoNote, 13*day - 5*time.Minute, 94 * time.Second, ""},
	{"storefront", "staging", 0, "succeeded", "j.devries", "", 13*day - 2*hour, 2 * time.Minute, ""},
	{"storefront", "production", 0, "succeeded", "", afterApproval, 0, 3 * time.Minute, ""},
	{"storefront", "dev", 1, "succeeded", schedulerActor, autoNote, 10*day - 4*time.Minute, 88 * time.Second, ""},
	{"storefront", "staging", 1, "succeeded", "d.kowalski", "", 10*day - 1*hour, 2 * time.Minute, ""},
	{"storefront", "production", 1, "succeeded", "", afterApproval, 0, 3 * time.Minute, ""},
	{"storefront", "dev", 2, "succeeded", schedulerActor, autoNote, 7*day - 6*time.Minute, 101 * time.Second, ""},
	{"storefront", "staging", 2, "failed", "d.kowalski", "", 7*day - 40*time.Minute, 41 * time.Second, manifestError},
	{"storefront", "dev", 3, "succeeded", schedulerActor, autoNote, 4*day - 5*time.Minute, 97 * time.Second, ""},
	{"storefront", "staging", 3, "succeeded", "c.santos", "retry after the api image was pushed", 4*day - 1*hour, 2 * time.Minute, ""},
	{"storefront", "dev", 4, "succeeded", schedulerActor, autoNote, 26*hour - 5*time.Minute, 92 * time.Second, ""},
	{"storefront", "staging", 4, "succeeded", "c.santos", "", 25 * hour, 2 * time.Minute, ""},
	{"storefront", "dev", 5, "pending", schedulerActor, autoNote, 8 * time.Minute, 0, ""},

	{"observability", "dev", 0, "succeeded", "t.becker", "", 16*day - 2*hour, 3 * time.Minute, ""},
	{"observability", "staging", 0, "succeeded", "t.becker", "", 15 * day, 3 * time.Minute, ""},
	{"observability", "prod", 0, "succeeded", "", afterApproval, 0, 4 * time.Minute, ""},
	{"observability", "dev", 2, "succeeded", "e.garcia", "30 day retention", 5*day - 2*hour, 3 * time.Minute, ""},
	{"observability", "staging", 2, "succeeded", "e.garcia", "", 5*day - 4*hour, 3 * time.Minute, ""},
	{"observability", "dev", 3, "failed", "p.oconnor", "trace sampling", 20 * time.Minute, 12 * time.Second, infraFetchError},

	{"edge-pos", "pilot", 0, "succeeded", schedulerActor, autoNote, 12*day - 6*time.Minute, 70 * time.Second, ""},
	{"edge-pos", "wave-1", 0, "succeeded", "", afterApproval, 0, 2 * time.Minute, ""},
	{"edge-pos", "wave-2", 0, "succeeded", "", afterApproval, 0, 2 * time.Minute, ""},
	{"edge-pos", "pilot", 1, "succeeded", schedulerActor, autoNote, 8*day - 5*time.Minute, 66 * time.Second, ""},
	{"edge-pos", "pilot", 2, "succeeded", schedulerActor, autoNote, 3*day - 6*time.Minute, 71 * time.Second, ""},
	{"edge-pos", "wave-1", 2, "succeeded", "", afterApproval, 0, 2 * time.Minute, ""},
	{"edge-pos", "pilot", 3, "succeeded", schedulerActor, autoNote, 20*hour - 4*time.Minute, 69 * time.Second, ""},
}

var demoApprovals = []gitApprovalSpec{
	{"storefront", "production", 0, "approved", "s.lindqvist", "Checked the search dashboards on staging", 21 * hour},
	{"storefront", "production", 1, "approved", "m.okafor", "", 19 * hour},
	{"storefront", "production", 3, "rejected", "l.moreau", "Payments freeze until the Q3 close; promote after Monday", 3 * hour},
	{"storefront", "production", 4, "pending", "", "", 0},
	{"observability", "prod", 0, "approved", "k.nakamura", "Grafana 11 dashboards verified on staging", 22 * hour},
	{"observability", "prod", 2, "pending", "", "", 0},
	{"edge-pos", "wave-1", 0, "approved", "l.moreau", "", 22 * hour},
	{"edge-pos", "wave-2", 0, "approved", "k.nakamura", "Amsterdam and Berlin fine overnight", 23 * hour},
	{"edge-pos", "wave-1", 1, "rejected", "t.becker", "Paris drawers are off by a cent on cash rounding; fix before the stores get it", 20 * hour},
	{"edge-pos", "wave-1", 2, "approved", "e.garcia", "Pilot store balanced its drawers two nights running", 5 * hour},
	{"edge-pos", "wave-2", 2, "pending", "", "", 0},
	{"edge-pos", "wave-1", 3, "pending", "", "", 0},
}

var demoPreviews = []gitPreviewSpec{
	{
		pipeline: "storefront-previews", pr: "482", title: "feat(checkout): one-page checkout with saved addresses",
		source: "feat/one-page-checkout", status: "active", hostKey: demoPreviewHostKey,
		pushes: []gitPushSpec{{"pr-482-a", 22 * hour}, {"pr-482-b", 3 * hour}},
	},
	{
		pipeline: "storefront-previews", pr: "485", title: "feat(search): facet counts on category pages",
		source: "feat/facet-counts", status: "failed", closedAgo: 4*hour + 40*time.Minute, tornDownBy: "failed-provisioning",
		lastError: "deploying stack {stack} failed: docker compose up failed: " +
			"service \"search-indexer\" refers to undefined volume facet-cache: invalid compose project",
		pushes: []gitPushSpec{{"pr-485-a", 5 * hour}},
	},
	{
		pipeline: "storefront-previews", pr: "479", title: "fix(cart): keep the coupon after a currency switch",
		source: "fix/coupon-currency", status: "expired", tornDownBy: "expired",
		pushes: []gitPushSpec{{"pr-479-a", 4 * day}, {"pr-479-b", 80 * hour}},
	},
	{
		pipeline: "storefront-previews", pr: "476", title: "chore(web): migrate the build to Vite 6",
		source: "chore/vite-6", status: "demolished", closedAgo: 7 * day, tornDownBy: "pr-merged",
		pushes: []gitPushSpec{{"pr-476-a", 9 * day}},
	},
}
