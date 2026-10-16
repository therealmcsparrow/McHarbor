// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package gitops

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Normalised pull-request actions. A forge sends a much larger vocabulary than
// the lifecycle cares about; everything else normalises to actionIgnore, which
// is answered with 200 so the forge's delivery log stays clean.
const (
	actionOpened      = "opened"
	actionReopened    = "reopened"
	actionSynchronize = "synchronize"
	actionClosed      = "closed"
	actionMerged      = "merged"
	actionIgnore      = "ignore"
)

// WebhookDelivery is one inbound forge request, carried as the bytes that were
// actually sent. The raw body is the thing signed, so it travels together with
// the headers that prove it rather than being re-marshalled from a struct.
type WebhookDelivery struct {
	RepoID      string
	Body        []byte
	Event       string
	HMACHeader  string
	TokenHeader string
}

// verifySignature checks a delivery against secret. GitHub and Gitea send an
// HMAC of the body; GitLab sends the secret itself, so both are compared in
// constant time and a delivery that carries neither is refused.
func (d WebhookDelivery) verifySignature(secret string) error {
	if secret == "" {
		return ErrWebhookSecretMissing
	}
	if sig := strings.TrimSpace(d.HMACHeader); sig != "" {
		sig = strings.TrimPrefix(sig, "sha256=")
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(d.Body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(expected), []byte(strings.ToLower(sig))) {
			return nil
		}
		return ErrWebhookSignatureInvalid
	}
	if token := strings.TrimSpace(d.TokenHeader); token != "" {
		if hmac.Equal([]byte(token), []byte(secret)) {
			return nil
		}
		return ErrWebhookSignatureInvalid
	}
	return ErrWebhookSignatureInvalid
}

// githubPullRequest is the subset of a GitHub or Gitea pull_request payload the
// lifecycle reads. Both forges use the same shape, which is why one decoder
// serves them.
type githubPullRequest struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Merged bool   `json:"merged"`
		State  string `json:"state"`
		Head   struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
}

// gitlabMergeRequest is the subset of a GitLab merge-request event the lifecycle
// reads. GitLab numbers merge requests per project with `iid`, not `id`.
type gitlabMergeRequest struct {
	ObjectKind       string `json:"object_kind"`
	ObjectAttributes struct {
		IID          int    `json:"iid"`
		Title        string `json:"title"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		Action       string `json:"action"`
		State        string `json:"state"`
		LastCommit   struct {
			ID string `json:"id"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
}

// normalizePullRequest turns a GitHub, GitLab, Gitea or native McHarbor body
// into the internal shape. The native form stays supported because it is what
// the CLI and the tests drive, and because a forge McHarbor has not learned yet
// can still be adapted by whatever sits in front of it.
func normalizePullRequest(d WebhookDelivery) (PullRequestInput, error) {
	var native PullRequestInput
	if err := json.Unmarshal(d.Body, &native); err == nil && native.PRNumber != "" {
		native.RepoID = orEmpty(d.RepoID, native.RepoID)
		native.Action = normalizeAction(native.Action, false)
		return native, nil
	}

	var gl gitlabMergeRequest
	if err := json.Unmarshal(d.Body, &gl); err == nil && gl.ObjectKind == "merge_request" {
		attrs := gl.ObjectAttributes
		return PullRequestInput{
			Action:       normalizeGitLabAction(attrs.Action, attrs.State),
			PRNumber:     strconv.Itoa(attrs.IID),
			PRTitle:      attrs.Title,
			SourceBranch: attrs.SourceBranch,
			TargetBranch: attrs.TargetBranch,
			CommitSHA:    attrs.LastCommit.ID,
			RepoID:       d.RepoID,
		}, nil
	}

	var gh githubPullRequest
	if err := json.Unmarshal(d.Body, &gh); err != nil {
		return PullRequestInput{}, fmt.Errorf("gitops: unrecognised webhook payload: %w", err)
	}
	number := gh.PullRequest.Number
	if number == 0 {
		number = gh.Number
	}
	if number == 0 {
		return PullRequestInput{}, errors.New("gitops: webhook payload carries no pull request number")
	}
	return PullRequestInput{
		Action:       normalizeAction(gh.Action, gh.PullRequest.Merged),
		PRNumber:     strconv.Itoa(number),
		PRTitle:      gh.PullRequest.Title,
		SourceBranch: gh.PullRequest.Head.Ref,
		TargetBranch: gh.PullRequest.Base.Ref,
		CommitSHA:    gh.PullRequest.Head.SHA,
		RepoID:       d.RepoID,
	}, nil
}

// normalizeAction maps a GitHub or Gitea action to the lifecycle's vocabulary.
// A forge never sends "merged": it sends "closed" with pull_request.merged set,
// which is the distinction the teardown reason depends on.
func normalizeAction(action string, merged bool) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "opened":
		return actionOpened
	case "reopened":
		return actionReopened
	case "synchronize", "synchronized", "updated":
		return actionSynchronize
	case "merged":
		return actionMerged
	case "closed":
		if merged {
			return actionMerged
		}
		return actionClosed
	default:
		return actionIgnore
	}
}

// normalizeGitLabAction maps GitLab's merge-request action vocabulary, which
// names the same five events differently and reports a merge through `action`
// rather than a flag on the object.
func normalizeGitLabAction(action, state string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "open":
		return actionOpened
	case "reopen":
		return actionReopened
	case "update":
		return actionSynchronize
	case "merge":
		return actionMerged
	case "close":
		return actionClosed
	default:
		if strings.EqualFold(state, "merged") {
			return actionMerged
		}
		return actionIgnore
	}
}
