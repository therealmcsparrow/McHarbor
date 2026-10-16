// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package dockerapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/volume"

	"github.com/therealmcsparrow/mcharbor/core/demo/estate"
)

func repoOf(tag string) string {
	if i := strings.LastIndex(tag, ":"); i > strings.LastIndex(tag, "/") {
		return tag[:i]
	}
	return tag
}

func (s *Server) images(w http.ResponseWriter, r *http.Request, rest string) {
	h := s.host
	switch {
	case rest == "/json" && r.Method == http.MethodGet:
		imgs, used := h.Images()
		f := parseFilters(r)
		out := []image.Summary{}
		for _, img := range imgs {
			if d := f["dangling"]; len(d) > 0 && (d[0] == "true") != (len(img.RepoTags) == 0) {
				continue
			}
			if refs := f["reference"]; len(refs) > 0 && !anyTagMatches(img.RepoTags, refs) {
				continue
			}
			if !queryBool(r, "all") && len(img.RepoTags) == 0 && used[img.ID] > 0 {
				continue
			}
			out = append(out, imageSummary(img, used[img.ID]))
		}
		writeJSON(w, http.StatusOK, out)
		return
	case rest == "/create" && r.Method == http.MethodPost:
		ref := r.URL.Query().Get("fromImage")
		if tag := r.URL.Query().Get("tag"); tag != "" && !strings.Contains(ref, "@") {
			ref += ":" + tag
		}
		s.pull(w, r, ref)
		return
	case rest == "/prune" && r.Method == http.MethodPost:
		all := false
		if d := parseFilters(r)["dangling"]; len(d) > 0 && d[0] == "false" {
			all = true
		}
		removed, reclaimed := h.PruneImages(all)
		rep := image.PruneReport{SpaceReclaimed: uint64(reclaimed)}
		for _, img := range removed {
			rep.ImagesDeleted = append(rep.ImagesDeleted, image.DeleteResponse{Deleted: img.ID})
		}
		writeJSON(w, http.StatusOK, rep)
		return
	case rest == "/search":
		writeJSON(w, http.StatusOK, []map[string]any{})
		return
	case rest == "/load" || rest == "/get":
		notInDemo(w)
		return
	}
	name := strings.TrimPrefix(rest, "/")
	switch {
	case r.Method == http.MethodDelete:
		img, err := h.RemoveImage(name, queryBool(r, "force"))
		if err != nil {
			writeEstateError(w, err, "image: "+name)
			return
		}
		out := []image.DeleteResponse{}
		for _, t := range img.RepoTags {
			out = append(out, image.DeleteResponse{Untagged: t})
		}
		out = append(out, image.DeleteResponse{Deleted: img.ID})
		writeJSON(w, http.StatusOK, out)
	case strings.HasSuffix(name, "/json"):
		ref := strings.TrimSuffix(name, "/json")
		img, err := h.Image(ref)
		if err != nil {
			writeEstateError(w, err, "image: "+ref)
			return
		}
		writeJSON(w, http.StatusOK, imageInspect(img))
	case strings.HasSuffix(name, "/history"):
		ref := strings.TrimSuffix(name, "/history")
		img, err := h.Image(ref)
		if err != nil {
			writeEstateError(w, err, "image: "+ref)
			return
		}
		writeJSON(w, http.StatusOK, imageHistory(img))
	case strings.HasSuffix(name, "/tag"):
		ref := strings.TrimSuffix(name, "/tag")
		tag := r.URL.Query().Get("repo")
		if t := r.URL.Query().Get("tag"); t != "" {
			tag += ":" + t
		}
		if err := h.TagImage(ref, tag); err != nil {
			writeEstateError(w, err, "image: "+ref)
			return
		}
		w.WriteHeader(http.StatusCreated)
	case strings.HasSuffix(name, "/push"):
		s.progress(w, r, strings.TrimSuffix(name, "/push"), "Pushed")
	default:
		notInDemo(w)
	}
}

func anyTagMatches(tags, refs []string) bool {
	for _, tag := range tags {
		for _, ref := range refs {
			if strings.Contains(tag, strings.TrimSuffix(ref, "*")) {
				return true
			}
		}
	}
	return false
}

func imageInspect(img estate.Image) image.InspectResponse {
	digests := []string{}
	if len(img.RepoTags) > 0 {
		digests = []string{repoOf(img.RepoTags[0]) + "@" + img.Digest}
	}
	tags := img.RepoTags
	if tags == nil {
		tags = []string{}
	}
	return image.InspectResponse{
		ID:           img.ID,
		RepoTags:     tags,
		RepoDigests:  digests,
		Created:      img.Created.UTC().Format(time.RFC3339Nano),
		Architecture: img.Arch,
		Os:           img.OS,
		Size:         img.Size,
		Author:       "",
		Config:       nil,
		RootFS:       image.RootFS{Type: "layers", Layers: layers(img)},
		Metadata:     image.Metadata{LastTagTime: img.Created},
	}
}

func layers(img estate.Image) []string {
	n := int(estate.StableHash(img.ID)%9) + 3
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("sha256:%064x", estate.StableHash(fmt.Sprint(img.ID, i)))
	}
	return out
}

func imageHistory(img estate.Image) []image.HistoryResponseItem {
	steps := []string{
		"/bin/sh -c #(nop) ADD file:5d673d25da3a14ce1f6cf66e4c7fd4f4b85a3759a9d93efb3fd9ff852b5b56e4 in / ",
		"/bin/sh -c #(nop)  CMD [\"bash\"]",
		"RUN /bin/sh -c apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/* # buildkit",
		"COPY /app /app # buildkit",
		"EXPOSE map[8080/tcp:{}]",
		"ENTRYPOINT [\"/entrypoint.sh\"]",
	}
	out := make([]image.HistoryResponseItem, 0, len(steps))
	for i := len(steps) - 1; i >= 0; i-- {
		item := image.HistoryResponseItem{ID: "<missing>", Created: img.Created.Add(-time.Duration(len(steps)-i) * time.Minute).Unix(), CreatedBy: steps[i], Size: img.Size / int64(len(steps))}
		if i == len(steps)-1 {
			item.ID = img.ID
			item.Tags = img.RepoTags
		}
		out = append(out, item)
	}
	return out
}

// pull streams the progress documents docker pull prints, then makes the image
// present so the container that asked for it can be created.
func (s *Server) pull(w http.ResponseWriter, r *http.Request, ref string) {
	img := s.host.PullImage(ref)
	s.progressFor(w, r, ref, img, "Pull complete")
}

func (s *Server) progress(w http.ResponseWriter, r *http.Request, ref, done string) {
	img, err := s.host.Image(ref)
	if err != nil {
		writeEstateError(w, err, "image: "+ref)
		return
	}
	s.progressFor(w, r, ref, img, done)
}

func (s *Server) progressFor(w http.ResponseWriter, r *http.Request, ref string, img estate.Image, done string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	emit := func(v map[string]any) bool {
		if err := enc.Encode(v); err != nil {
			return false
		}
		flush(w)
		return true
	}
	tag := "latest"
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		tag = ref[i+1:]
	}
	if !emit(map[string]any{"status": "Pulling from " + repoOf(ref), "id": tag}) {
		return
	}
	for _, layer := range layers(img)[:3] {
		id := strings.TrimPrefix(layer, "sha256:")[:12]
		for pct := 0; pct <= 100; pct += 50 {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(60 * time.Millisecond):
			}
			if !emit(map[string]any{"status": "Downloading", "id": id, "progressDetail": map[string]int64{"current": img.Size / 3 * int64(pct) / 100, "total": img.Size / 3}}) {
				return
			}
		}
		if !emit(map[string]any{"status": done, "id": id}) {
			return
		}
	}
	if emit(map[string]any{"status": "Digest: " + img.Digest}) {
		emit(map[string]any{"status": "Status: Image is up to date for " + ref})
	}
}

func volumeDoc(v estate.Volume) volume.Volume {
	labels := v.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return volume.Volume{
		CreatedAt:  v.Created.UTC().Format(time.RFC3339),
		Driver:     v.Driver,
		Labels:     labels,
		Mountpoint: "/var/lib/docker/volumes/" + v.Name + "/_data",
		Name:       v.Name,
		Options:    map[string]string{},
		Scope:      "local",
	}
}

func (s *Server) volumes(w http.ResponseWriter, r *http.Request, rest string) {
	h := s.host
	switch {
	case rest == "" && r.Method == http.MethodGet:
		vols, refs := h.Volumes()
		f := parseFilters(r)
		out := volume.ListResponse{Volumes: []volume.Volume{}, Warnings: []string{}}
		for _, v := range vols {
			if d := f["dangling"]; len(d) > 0 && (d[0] == "true") != (refs[v.Name] == 0) {
				continue
			}
			if !labelsMatch(v.Labels, f["label"]) {
				continue
			}
			doc := volumeDoc(v)
			out.Volumes = append(out.Volumes, doc)
		}
		writeJSON(w, http.StatusOK, out)
	case rest == "/create" && r.Method == http.MethodPost:
		var body volume.CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid volume config")
			return
		}
		writeJSON(w, http.StatusCreated, volumeDoc(h.CreateVolume(body.Name, body.Driver, body.Labels)))
	case rest == "/prune" && r.Method == http.MethodPost:
		removed, reclaimed := h.PruneVolumes()
		writeJSON(w, http.StatusOK, volume.PruneReport{VolumesDeleted: removed, SpaceReclaimed: uint64(reclaimed)})
	case r.Method == http.MethodDelete:
		name := strings.TrimPrefix(rest, "/")
		if err := h.RemoveVolume(name, queryBool(r, "force")); err != nil {
			writeEstateError(w, err, "volume: "+name)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet:
		name := strings.TrimPrefix(rest, "/")
		v, err := h.Volume(name)
		if err != nil {
			writeEstateError(w, err, "volume: "+name)
			return
		}
		_, refs := h.Volumes()
		doc := volumeDoc(v)
		doc.UsageData = &volume.UsageData{Size: v.SizeBytes, RefCount: int64(refs[v.Name])}
		writeJSON(w, http.StatusOK, doc)
	default:
		notInDemo(w)
	}
}

func networkDoc(n estate.Network, attached map[string]estate.Container) network.Inspect {
	doc := network.Inspect{
		Name:       n.Name,
		ID:         n.ID,
		Created:    n.Created,
		Scope:      n.Scope,
		Driver:     n.Driver,
		EnableIPv4: true,
		Internal:   n.Internal,
		Attachable: n.Driver == "bridge" && n.Name != "bridge",
		Options:    map[string]string{},
		Labels:     n.Labels,
		Containers: map[string]network.EndpointResource{},
	}
	if doc.Labels == nil {
		doc.Labels = map[string]string{}
	}
	if n.Subnet != "" {
		doc.IPAM = network.IPAM{Driver: "default", Config: []network.IPAMConfig{{Subnet: ipPrefix(n.Subnet), Gateway: ipAddr(n.Gateway)}}}
	}
	for id, c := range attached {
		ip := c.Networks[n.Name]
		doc.Containers[id] = network.EndpointResource{Name: c.Name, EndpointID: fmt.Sprintf("%064x", estate.StableHash(id+n.Name)), MacAddress: hwAddr(macOf(id + n.Name)), IPv4Address: ipPrefix(ip + "/16")}
	}
	return doc
}

func (s *Server) networks(w http.ResponseWriter, r *http.Request, rest string) {
	h := s.host
	switch {
	case rest == "" && r.Method == http.MethodGet:
		f := parseFilters(r)
		out := []network.Summary{}
		for _, n := range h.Networks() {
			if !labelsMatch(n.Labels, f["label"]) {
				continue
			}
			if names := f["name"]; len(names) > 0 && !anySubstring(n.Name, names) {
				continue
			}
			out = append(out, network.Summary{Network: networkDoc(n, nil).Network})
		}
		writeJSON(w, http.StatusOK, out)
	case rest == "/create" && r.Method == http.MethodPost:
		var body network.CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid network config")
			return
		}
		n, err := h.CreateNetwork(body.Name, body.Driver, body.Internal, body.Labels)
		if err != nil {
			writeError(w, http.StatusConflict, "network with name "+body.Name+" already exists")
			return
		}
		writeJSON(w, http.StatusCreated, network.CreateResponse{ID: n.ID})
	case rest == "/prune" && r.Method == http.MethodPost:
		writeJSON(w, http.StatusOK, network.PruneReport{NetworksDeleted: h.PruneNetworks()})
	default:
		ref, action, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
		switch {
		case action == "" && r.Method == http.MethodGet:
			n, attached, err := h.Network(ref)
			if err != nil {
				writeEstateError(w, err, "network: "+ref)
				return
			}
			writeJSON(w, http.StatusOK, networkDoc(n, attached))
		case action == "" && r.Method == http.MethodDelete:
			if err := h.RemoveNetwork(ref); err != nil {
				writeEstateError(w, err, "network: "+ref)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case action == "connect" || action == "disconnect":
			var body struct{ Container string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, "invalid request")
				return
			}
			var err error
			if action == "connect" {
				err = h.ConnectNetwork(ref, body.Container)
			} else {
				err = h.DisconnectNetwork(ref, body.Container)
			}
			if err != nil {
				writeEstateError(w, err, "network: "+ref)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			notInDemo(w)
		}
	}
}
