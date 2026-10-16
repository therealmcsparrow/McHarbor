// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package reconciler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/envredact"
)

// ErrNotExpressible is returned when none of a drift can be written back as
// intent: a digest, a mount or a compose change has no field in the document.
var ErrNotExpressible = errors.New("reconciler: this drift cannot be expressed as a change to the document")

// PullRequestResult is what writing drift back to Git did.
type PullRequestResult struct {
	Branch         string   `json:"branch"`
	PullRequestURL string   `json:"pullRequestUrl,omitempty"`
	Provider       string   `json:"provider,omitempty"`
	ProviderError  string   `json:"providerError,omitempty"`
	Changed        []string `json:"changed"`
	Skipped        []string `json:"skipped"`
}

// HTTPDoer is the part of *http.Client pull request creation uses.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// OpenPullRequest proposes making Git match reality for a Git-managed desired
// state: its document is rewritten from the recorded drift, committed to a new
// branch and pushed, and a pull request is opened when the host is recognised.
func (s *SourceSyncer) OpenPullRequest(ctx context.Context, ds *DesiredState, httpc HTTPDoer) (*PullRequestResult, error) {
	if ds.SourceID == "" {
		return nil, errors.New("reconciler: desired state is not managed by Git")
	}
	if ds.LastReport == nil || !ds.LastReport.HasDrift {
		return nil, ErrNoDrift
	}
	if ds.Kind == KindStack {
		return nil, ErrNotExpressible
	}
	src, err := s.Get(ctx, ds.SourceID)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, errors.New("reconciler: the Git source no longer exists")
	}
	dir, repo, auth, token, err := s.clone(ctx, src)
	if err != nil {
		return nil, err
	}
	defer removeDir(s.logger, dir)

	file, index, err := splitSourcePath(ds.SourcePath)
	if err != nil {
		return nil, err
	}
	res := &PullRequestResult{Changed: []string{}, Skipped: []string{}}
	var touched []string
	if ds.Kind == KindEnvironment {
		touched, err = writeUndeclared(dir, src.Path, ds, res)
	} else {
		err = rewriteDoc(filepath.Join(dir, filepath.FromSlash(file)), index, ds.LastReport, res)
		touched = []string{file}
	}
	if err != nil {
		return nil, err
	}
	if len(res.Changed) == 0 {
		return res, ErrNotExpressible
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("opening the work tree: %w", err)
	}
	res.Branch = fmt.Sprintf("mcharbor/drift-%s-%d", ds.ID, s.now().Unix())
	if err := wt.Checkout(&gogit.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(res.Branch), Create: true, Keep: true}); err != nil {
		return nil, fmt.Errorf("creating branch: %w", err)
	}
	for _, f := range touched {
		if _, err := wt.Add(f); err != nil {
			return nil, fmt.Errorf("staging %s: %w", f, err)
		}
	}
	title := fmt.Sprintf("Accept drift on %s", ds.Name)
	body := "McHarbor found this desired state drifted and proposes changing the intent to match what is running.\n\n" +
		"Changed:\n- " + strings.Join(res.Changed, "\n- ")
	if len(res.Skipped) > 0 {
		body += "\n\nNot expressible here, left as drift:\n- " + strings.Join(res.Skipped, "\n- ")
	}
	if _, err := wt.Commit(title+"\n\n"+body, &gogit.CommitOptions{
		Author: &object.Signature{Name: "McHarbor", Email: "mcharbor@localhost", When: s.now()},
	}); err != nil {
		return nil, fmt.Errorf("committing: %w", err)
	}
	pushCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	ref := gitconfig.RefSpec("refs/heads/" + res.Branch + ":refs/heads/" + res.Branch)
	if err := repo.PushContext(pushCtx, &gogit.PushOptions{RemoteName: "origin", Auth: auth, RefSpecs: []gitconfig.RefSpec{ref}}); err != nil {
		return nil, fmt.Errorf("pushing %s: %w", res.Branch, err)
	}

	remote, err := repo.Remote("origin")
	if err != nil || len(remote.Config().URLs) == 0 {
		res.ProviderError = "no remote URL"
		return res, nil
	}
	prURL, provider, prErr := createPullRequest(ctx, httpc, remote.Config().URLs[0], token, res.Branch, src.Branch, title, body)
	res.PullRequestURL, res.Provider = prURL, provider
	if prErr != nil {
		res.ProviderError = prErr.Error()
	}
	return res, nil
}

func splitSourcePath(p string) (string, int, error) {
	file, idx, ok := strings.Cut(p, "#")
	n, err := strconv.Atoi(idx)
	if !ok || err != nil || n < 0 || file == "" {
		return "", 0, fmt.Errorf("reconciler: source path %q is malformed", p)
	}
	return file, n, nil
}

// rewriteDoc edits one document in place through the YAML node tree, so the
// file's comments and key order survive; a JSON file is written back as JSON.
func rewriteDoc(file string, index int, report *DriftReport, res *PullRequestResult) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Base(file), err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parsing %s: %w", filepath.Base(file), err)
	}
	if len(root.Content) == 0 {
		return fmt.Errorf("parsing %s: the file has no content", filepath.Base(file))
	}
	doc := root.Content[0]
	if list := mapValue(doc, "desiredStates"); list != nil {
		if list.Kind != yaml.SequenceNode || index >= len(list.Content) {
			return fmt.Errorf("document %d is no longer in %s", index+1, filepath.Base(file))
		}
		doc = list.Content[index]
	} else if index != 0 {
		return fmt.Errorf("document %d is no longer in %s", index+1, filepath.Base(file))
	}
	for _, d := range report.Diffs {
		if applyDiffToDoc(doc, d) {
			res.Changed = append(res.Changed, d.Field)
		} else {
			res.Skipped = append(res.Skipped, d.Field)
		}
	}
	var out []byte
	if strings.EqualFold(filepath.Ext(file), ".json") {
		var v any
		if err := root.Decode(&v); err != nil {
			return fmt.Errorf("re-reading %s: %w", filepath.Base(file), err)
		}
		if out, err = json.MarshalIndent(v, "", "  "); err != nil {
			return fmt.Errorf("encoding %s: %w", filepath.Base(file), err)
		}
		out = append(out, '\n')
	} else {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(&root); err != nil {
			return fmt.Errorf("encoding %s: %w", filepath.Base(file), err)
		}
		if err := enc.Close(); err != nil {
			return fmt.Errorf("encoding %s: %w", filepath.Base(file), err)
		}
		out = buf.Bytes()
	}
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	return os.WriteFile(file, out, info.Mode().Perm())
}

// applyDiffToDoc sets the document field a diff names to the value observed.
func applyDiffToDoc(doc *yaml.Node, d DriftDiff) bool {
	switch {
	case d.Class == ClassImage:
		return false
	case d.Field == "image":
		setScalar(doc, []string{"imageRef"}, d.Actual, "!!str")
	case d.Field == "restartPolicy":
		v := d.Actual
		if v == "none" {
			v = "no"
		}
		setScalar(doc, []string{"restartPolicy"}, v, "!!str")
	case d.Field == "status":
		v := "stopped"
		if d.Actual == "running" {
			v = "running"
		}
		setScalar(doc, []string{"desiredStatus"}, v, "!!str")
	case d.Field == "existence":
		v := "removed"
		if d.Actual == "exists" {
			v = "running"
		}
		setScalar(doc, []string{"desiredStatus"}, v, "!!str")
	case strings.HasPrefix(d.Field, "env.") && d.Actual == envredact.Marker:
		// The live value was withheld from the report; writing the marker into the source
		// would replace a credential with bullets, so this field stays a manual change.
		return false
	case strings.HasPrefix(d.Field, "env."), strings.HasPrefix(d.Field, "label."):
		group, key, _ := strings.Cut(d.Field, ".")
		section := map[string]string{"env": "env", "label": "labels"}[group]
		if d.Actual == "missing" {
			deleteKey(doc, []string{"config", section, key})
		} else {
			setScalar(doc, []string{"config", section, key}, d.Actual, "!!str")
		}
	case d.Field == "memoryBytes", d.Field == "nanoCpus":
		if _, err := strconv.ParseInt(d.Actual, 10, 64); err != nil {
			return false
		}
		setScalar(doc, []string{"config", d.Field}, d.Actual, "!!int")
	default:
		return false
	}
	return true
}

func mapValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func setScalar(n *yaml.Node, keys []string, value, tag string) {
	for i, k := range keys {
		child := mapValue(n, k)
		last := i == len(keys)-1
		if child == nil {
			child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			if last {
				child = &yaml.Node{Kind: yaml.ScalarNode}
			}
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, child)
		}
		if last {
			child.Kind, child.Tag, child.Value, child.Content = yaml.ScalarNode, tag, value, nil
			return
		}
		n = child
	}
}

func deleteKey(n *yaml.Node, keys []string) {
	for i, k := range keys {
		if i == len(keys)-1 {
			if n == nil || n.Kind != yaml.MappingNode {
				return
			}
			for j := 0; j+1 < len(n.Content); j += 2 {
				if n.Content[j].Value == k {
					n.Content = append(n.Content[:j], n.Content[j+2:]...)
					return
				}
			}
			return
		}
		n = mapValue(n, k)
	}
}

// writeUndeclared declares each undeclared container in a new file, observe
// mode, so accepting the drift makes them intent instead of noise.
func writeUndeclared(dir, sub string, ds *DesiredState, res *PullRequestResult) ([]string, error) {
	var docs []sourceDoc
	for _, d := range ds.LastReport.Diffs {
		name, ok := strings.CutPrefix(d.Field, "undeclared.")
		if !ok {
			res.Skipped = append(res.Skipped, d.Field)
			continue
		}
		docs = append(docs, sourceDoc{Name: name, Kind: KindContainer, ContainerName: name, ImageRef: d.Actual,
			DesiredStatus: "running", Mode: ModeObserve, Description: "Declared from drift on " + ds.Name})
		res.Changed = append(res.Changed, d.Field)
	}
	if len(docs) == 0 {
		return nil, nil
	}
	rel := strings.TrimPrefix(filepath.ToSlash(filepath.Join(sub, "declared-"+strconv.FormatInt(time.Now().Unix(), 10)+".yaml")), "/")
	raw, err := yaml.Marshal(docFile{DesiredStates: docs})
	if err != nil {
		return nil, fmt.Errorf("encoding declarations: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), raw, 0o644); err != nil {
		return nil, fmt.Errorf("writing declarations: %w", err)
	}
	return []string{rel}, nil
}

// createPullRequest opens a pull request on GitHub, GitLab or Gitea/Forgejo.
// An unrecognised host is not an error: the branch is pushed and the operator
// opens the request themselves.
func createPullRequest(ctx context.Context, httpc HTTPDoer, remoteURL, token, head, base, title, body string) (string, string, error) {
	u, err := url.Parse(remoteURL)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("remote %q is not an HTTP URL", remoteURL)
	}
	if token == "" {
		return "", "", errors.New("the repository has no token to open a pull request with")
	}
	repoPath := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	scheme := u.Scheme
	if scheme != "http" {
		scheme = "https"
	}
	var endpoint, provider string
	var payload map[string]string
	header := map[string]string{}
	switch {
	case u.Host == "github.com":
		provider, endpoint = "github", "https://api.github.com/repos/"+repoPath+"/pulls"
		payload = map[string]string{"title": title, "head": head, "base": base, "body": body}
		header["Authorization"], header["Accept"] = "Bearer "+token, "application/vnd.github+json"
	case strings.Contains(u.Host, "gitlab"):
		provider = "gitlab"
		endpoint = scheme + "://" + u.Host + "/api/v4/projects/" + url.PathEscape(repoPath) + "/merge_requests"
		payload = map[string]string{"title": title, "source_branch": head, "target_branch": base, "description": body}
		header["PRIVATE-TOKEN"] = token
	default:
		provider = "gitea"
		endpoint = scheme + "://" + u.Host + "/api/v1/repos/" + repoPath + "/pulls"
		payload = map[string]string{"title": title, "head": head, "base": base, "body": body}
		header["Authorization"] = "token " + token
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", provider, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", provider, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return "", provider, fmt.Errorf("calling %s: %w", provider, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			slog.Debug("reconciler: closing forge response body failed", "error", cerr)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", provider, fmt.Errorf("reading %s response: %w", provider, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return "", provider, fmt.Errorf("%s answered %d: %s", provider, resp.StatusCode, msg)
	}
	var out struct {
		HTMLURL string `json:"html_url"`
		WebURL  string `json:"web_url"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", provider, fmt.Errorf("decoding %s response: %w", provider, err)
	}
	if out.HTMLURL != "" {
		return out.HTMLURL, provider, nil
	}
	return out.WebURL, provider, nil
}
