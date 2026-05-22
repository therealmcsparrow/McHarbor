// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package images

// PullRequest is the JSON body for POST /images.
type PullRequest struct {
	Image string `json:"image"`
	Tag   string `json:"tag,omitempty"`
}

// TagRequest is the JSON body for POST /images/{id}/tag.
type TagRequest struct {
	Repo string `json:"repo"`
	Tag  string `json:"tag"`
}

// ImageSummary is a simplified image info for list responses.
// JSON tags use PascalCase to match Docker API convention and frontend types.
type ImageSummary struct {
	ID          string            `json:"Id"`
	ParentID    string            `json:"ParentId"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Created     int64             `json:"Created"`
	Size        int64             `json:"Size"`
	SharedSize  int64             `json:"SharedSize"`
	Containers  int64             `json:"Containers"`
	Labels      map[string]string `json:"Labels"`
}
