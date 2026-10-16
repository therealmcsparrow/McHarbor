// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"context"
	"github.com/moby/moby/api/types/container"
	"net/http"
	"strings"
)

// mountRef is the part of a container mount an audit row needs: where the data
// lives, not how it is wired.
type mountRef struct {
	Destination string
	Name        string
	Type        string
}

// details renders a mount as audit detail fields. Volume name and bind source
// both land in `volume=` because both answer the only question the reader has:
// which piece of host-side storage was touched.
func (m mountRef) details() string {
	out := " mount=" + m.Destination
	if m.Name != "" {
		out += " volume=" + m.Name
	}
	if m.Type != "" {
		out += " type=" + m.Type
	}
	return out
}

// mountForPath returns the most specific mount containing p.
//
// Most specific matters: a container with `/data` and `/data/pgdata` mounted
// separately would otherwise attribute every write to the outer volume, and the
// audit row would name storage the change never reached.
func mountForPath(mounts []container.MountPoint, p string) (mountRef, bool) {
	cleaned := cleanContainerPath(p)
	best := mountRef{}
	found := false
	for _, mp := range mounts {
		dest := cleanContainerPath(mp.Destination)
		if cleaned != dest && !strings.HasPrefix(cleaned, strings.TrimSuffix(dest, "/")+"/") {
			continue
		}
		if found && len(dest) <= len(best.Destination) {
			continue
		}
		name := mp.Name
		if name == "" {
			name = mp.Source
		}
		best = mountRef{Destination: dest, Name: name, Type: string(mp.Type)}
		found = true
	}
	return best, found
}

// fileAuditContext resolves, from a single inspect, the container name an audit
// row is labelled with and the mount fields describing where the path lives.
// The suffix is "" when every path is container-local.
//
// One inspect is the same price every audited container mutation already pays
// for its name, so folding the mount lookup into it adds no round trip.
func (h *Handler) fileAuditContext(
	ctx context.Context, envID, id string, paths ...string,
) (name, suffix string) {
	info, err := h.svc.Inspect(ctx, envID, id)
	if err != nil {
		return shortContainerID(id), ""
	}
	name = normalizeContainerName(info.Name, id)
	for _, p := range paths {
		if ref, ok := mountForPath(info.Mounts, p); ok {
			return name, ref.details()
		}
	}
	return name, ""
}

// auditMountMutation records a change to a path, naming the mount or volume it
// landed in when there is one.
func (h *Handler) auditMountMutation(r *http.Request, envID, id, action, details string, paths ...string) {
	name, suffix := h.fileAuditContext(r.Context(), envID, id, paths...)
	h.logContainerAuditWithName(r, envID, action, id, name, details+suffix)
}

// auditMountRead logs a read of a path that lives inside a mount or volume.
//
// Container-local reads are deliberately not logged: they are ephemeral, they
// are the bulk of file-browser traffic, and a log nobody can read through is a
// log nobody reads. Data that outlives the container is the thing worth a row.
func (h *Handler) auditMountRead(r *http.Request, envID, id, action, details string, paths ...string) {
	name, suffix := h.fileAuditContext(r.Context(), envID, id, paths...)
	if suffix == "" {
		return
	}
	h.logContainerAuditWithName(r, envID, action, id, name, details+suffix)
}
