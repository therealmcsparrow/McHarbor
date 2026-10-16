// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import "testing"

// TestForgePayloadsNormalise proves a real GitHub, GitLab or Gitea body reaches
// the lifecycle without a translation shim outside McHarbor. Before this, the
// endpoint only understood a bespoke flat struct nothing in the world sends.
func TestForgePayloadsNormalise(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		expect PullRequestInput
	}{
		{
			name: "github opened",
			body: `{"action":"opened","number":31,"pull_request":{"number":31,"title":"Add login",
			        "head":{"ref":"feature/login","sha":"aaa111"},"base":{"ref":"main"},"merged":false}}`,
			expect: PullRequestInput{Action: actionOpened, PRNumber: "31", PRTitle: "Add login",
				SourceBranch: "feature/login", TargetBranch: "main", CommitSHA: "aaa111", RepoID: "repo-1"},
		},
		{
			name: "github closed without a merge",
			body: `{"action":"closed","pull_request":{"number":31,"merged":false,
			        "head":{"ref":"f","sha":"bbb"},"base":{"ref":"main"}}}`,
			expect: PullRequestInput{Action: actionClosed, PRNumber: "31",
				SourceBranch: "f", TargetBranch: "main", CommitSHA: "bbb", RepoID: "repo-1"},
		},
		{
			name: "github closed with a merge is a merge",
			body: `{"action":"closed","pull_request":{"number":31,"merged":true,
			        "head":{"ref":"f","sha":"bbb"},"base":{"ref":"main"}}}`,
			expect: PullRequestInput{Action: actionMerged, PRNumber: "31",
				SourceBranch: "f", TargetBranch: "main", CommitSHA: "bbb", RepoID: "repo-1"},
		},
		{
			name: "github label change is ignored",
			body: `{"action":"labeled","pull_request":{"number":31,
			        "head":{"ref":"f","sha":"bbb"},"base":{"ref":"main"}}}`,
			expect: PullRequestInput{Action: actionIgnore, PRNumber: "31",
				SourceBranch: "f", TargetBranch: "main", CommitSHA: "bbb", RepoID: "repo-1"},
		},
		{
			name: "gitlab merge request opened",
			body: `{"object_kind":"merge_request","object_attributes":{"iid":7,"title":"Fix it",
			        "source_branch":"fix","target_branch":"main","action":"open","state":"opened",
			        "last_commit":{"id":"ccc222"}}}`,
			expect: PullRequestInput{Action: actionOpened, PRNumber: "7", PRTitle: "Fix it",
				SourceBranch: "fix", TargetBranch: "main", CommitSHA: "ccc222", RepoID: "repo-1"},
		},
		{
			name: "gitlab merge",
			body: `{"object_kind":"merge_request","object_attributes":{"iid":7,"source_branch":"fix",
			        "target_branch":"main","action":"merge","state":"merged","last_commit":{"id":"ccc"}}}`,
			expect: PullRequestInput{Action: actionMerged, PRNumber: "7",
				SourceBranch: "fix", TargetBranch: "main", CommitSHA: "ccc", RepoID: "repo-1"},
		},
		{
			name: "gitea synchronized",
			body: `{"action":"synchronized","number":3,"pull_request":{"number":3,"title":"WIP",
			        "head":{"ref":"wip","sha":"ddd"},"base":{"ref":"develop"}}}`,
			expect: PullRequestInput{Action: actionSynchronize, PRNumber: "3", PRTitle: "WIP",
				SourceBranch: "wip", TargetBranch: "develop", CommitSHA: "ddd", RepoID: "repo-1"},
		},
		{
			name: "native mcharbor body",
			body: `{"action":"opened","prNumber":"5","prTitle":"T","sourceBranch":"s","targetBranch":"main","commitSha":"eee"}`,
			expect: PullRequestInput{Action: actionOpened, PRNumber: "5", PRTitle: "T",
				SourceBranch: "s", TargetBranch: "main", CommitSHA: "eee", RepoID: "repo-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizePullRequest(WebhookDelivery{RepoID: "repo-1", Body: []byte(tc.body)})
			if err != nil {
				t.Fatalf("normalise: %v", err)
			}
			if got != tc.expect {
				t.Fatalf("got %+v\nwant %+v", got, tc.expect)
			}
		})
	}
}

// TestSanitizeHostLabel keeps a branch name from reaching a hostname or a
// container name with anything either of them refuses.
func TestSanitizeHostLabel(t *testing.T) {
	cases := map[string]string{
		"feature/Login_Page":  "feature-login-page",
		"  Release/2026.09  ": "release-2026-09",
		"---":                 "",
		"a/b//c":              "a-b-c",
	}
	for in, want := range cases {
		if got := sanitizeHostLabel(in); got != want {
			t.Fatalf("sanitizeHostLabel(%q) = %q, want %q", in, got, want)
		}
	}
	long := sanitizeHostLabel(string(make([]byte, 200)) + "x")
	if len(long) > 63 {
		t.Fatalf("a label of %d octets exceeds the DNS limit", len(long))
	}
}
