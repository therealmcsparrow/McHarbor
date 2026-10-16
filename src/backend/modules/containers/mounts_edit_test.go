// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package containers

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

// TestEditedMountsReplaceTheContainersMounts guards what a saved mount list becomes: binds and
// volumes as Binds strings (so a missing host directory is still created), tmpfs keeping its
// creation options, and every earlier mount — structured ones included — gone.
func TestEditedMountsReplaceTheContainersMounts(t *testing.T) {
	hc := &container.HostConfig{
		Binds:  []string{"/old:/data"},
		Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: "old", Target: "/old"}},
		Tmpfs:  map[string]string{"/run/cache": "size=64m"},
	}
	applyMountEdits(hc, []MountEdit{
		{Type: "bind", Source: "/mnt/tank/ha", Target: "/config", Propagation: "rslave"},
		{Type: "volume", Source: "ha-media", Target: "/media", ReadOnly: true},
		{Type: "tmpfs", Target: "/run/cache"},
	})
	if want := []string{"/mnt/tank/ha:/config:rslave", "ha-media:/media:ro"}; !reflect.DeepEqual(hc.Binds, want) {
		t.Errorf("binds = %v, want %v", hc.Binds, want)
	}
	if hc.Mounts != nil {
		t.Errorf("structured mounts survived: %v", hc.Mounts)
	}
	if hc.Tmpfs["/run/cache"] != "size=64m" {
		t.Errorf("tmpfs options lost: %v", hc.Tmpfs)
	}
}

func TestMountEditsAreValidatedBeforeTheRecreate(t *testing.T) {
	bad := map[string]MountEdit{
		"relative target":   {Type: "bind", Source: "/a", Target: "data"},
		"root target":       {Type: "bind", Source: "/a", Target: "/"},
		"relative source":   {Type: "bind", Source: "data", Target: "/data"},
		"colon in source":   {Type: "bind", Source: "/a:/b", Target: "/data"},
		"bad volume name":   {Type: "volume", Source: "../x", Target: "/data"},
		"unknown type":      {Type: "npipe", Source: "/a", Target: "/data"},
		"bogus propagation": {Type: "bind", Source: "/a", Target: "/data", Propagation: "everywhere"},
		"secrets mount":     {Type: "bind", Source: "/a", Target: "/run/secrets"},
	}
	for name, m := range bad {
		if err := validateMountEdits([]MountEdit{m}); !errors.Is(err, ErrInvalidMount) {
			t.Errorf("%s: err = %v, want ErrInvalidMount", name, err)
		}
	}
	dup := []MountEdit{{Type: "bind", Source: "/a", Target: "/data"}, {Type: "volume", Source: "v", Target: "/data"}}
	if err := validateMountEdits(dup); !errors.Is(err, ErrInvalidMount) {
		t.Errorf("duplicate target accepted: %v", err)
	}
	ok := []MountEdit{{Type: "bind", Source: "/srv/x", Target: "/x"}, {Type: "tmpfs", Target: "/tmp"}}
	if err := validateMountEdits(ok); err != nil {
		t.Errorf("valid mounts refused: %v", err)
	}
}

// TestRecreateEditRequestKeepsTheSharedFieldsFlat: the editor sends one flat object, so the
// embedded RecreateRequest must decode from the top level next to `mounts`.
func TestRecreateEditRequestKeepsTheSharedFieldsFlat(t *testing.T) {
	var req RecreateEditRequest
	if err := json.Unmarshal([]byte(`{"image":"nginx","mounts":[{"type":"tmpfs","target":"/tmp"}]}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.Image != "nginx" || req.Mounts == nil || len(*req.Mounts) != 1 {
		t.Errorf("decoded %+v", req)
	}
}
