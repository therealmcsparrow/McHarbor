// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor/core/plugin"
)

// Row scanning and the small conversions around it.
//
// SQLite stores timestamps as RFC3339 text in this project, and the plugin DB
// handle hands back a narrow Rows interface with no sql.NullString support, so
// every nullable column is read through COALESCE in the query and lands here as
// a plain string. That keeps each Scan target a plain type — and a column that
// forgets the COALESCE fails the whole read at runtime rather than at compile
// time, so new queries copy an existing SELECT rather than being written fresh.

func scanEnvironment(rows plugin.Rows) (ComputeEnvironment, error) {
	var env ComputeEnvironment
	var enabled int
	var capabilities, probedAt, created, updated string

	if err := rows.Scan(&env.ID, &env.EnvironmentID, &enabled, &capabilities, &probedAt,
		&env.DefaultPoolID, &env.DefaultNetworkID, &created, &updated); err != nil {
		return ComputeEnvironment{}, err
	}

	env.ComputeEnabled = enabled == 1
	env.ProbedAt = parseTimePtr(probedAt)
	env.CreatedAt = parseTime(created)
	env.UpdatedAt = parseTime(updated)

	if capabilities != "" {
		var capability Capability
		if err := json.Unmarshal([]byte(capabilities), &capability); err == nil {
			env.Capability = &capability
		}
		// A report that will not parse is treated as absent rather than as an
		// error: it means an older agent wrote a shape this build does not
		// know, and the fix is to probe again — which the UI offers anyway.
	}
	return env, nil
}

func scanPool(rows plugin.Rows) (Pool, error) {
	var pool Pool
	var isDefault int
	var sampledAt, created, updated string

	if err := rows.Scan(&pool.ID, &pool.EnvironmentID, &pool.Name, &pool.Backend,
		&pool.Target, &isDefault, &pool.CapacityBytes, &pool.UsedBytes,
		&pool.AllocatedBytes, &pool.VolumeCount, &sampledAt, &pool.State,
		&pool.LastError, &created, &updated); err != nil {
		return Pool{}, err
	}

	pool.IsDefault = isDefault == 1
	// Derived rather than stored: instant cloning is a property of the backend,
	// and a column could disagree with it.
	pool.Instant = InstantBackends[pool.Backend]
	pool.SampledAt = parseTimePtr(sampledAt)
	pool.CreatedAt = parseTime(created)
	pool.UpdatedAt = parseTime(updated)
	return pool, nil
}

func scanNetwork(rows plugin.Rows) (Network, error) {
	var network Network
	var dhcp, nat, hostAccess, ipv6, isDefault int
	var created, updated string

	if err := rows.Scan(&network.ID, &network.EnvironmentID, &network.Name, &network.Mode,
		&network.Bridge, &network.VLANTag, &network.SubnetCIDR, &network.GatewayIP,
		&dhcp, &network.DHCPRangeStart, &network.DHCPRangeEnd, &nat,
		&network.UplinkLink, &network.LinkMode, &hostAccess, &network.HostAccessIP,
		&ipv6, &network.HostAccessIPv6, &network.HostLinkName,
		&isDefault, &network.State, &network.LastError,
		&created, &updated); err != nil {
		return Network{}, err
	}

	network.DHCPEnabled = dhcp == 1
	network.NATEnabled = nat == 1
	network.HostAccess = hostAccess == 1
	network.IPv6Enabled = ipv6 == 1
	network.IsDefault = isDefault == 1
	network.CreatedAt = parseTime(created)
	network.UpdatedAt = parseTime(updated)
	return network, nil
}

func scanImage(rows plugin.Rows) (Image, error) {
	var image Image
	var builtin int
	var created, updated, uploaded string

	if err := rows.Scan(&image.ID, &image.Kind, &image.Distro, &image.Release,
		&image.Arch, &image.Variant, &image.DisplayName, &image.SourceURL,
		&image.ChecksumURL, &image.Checksum, &image.ChecksumAlgo,
		&image.SignatureURL, &image.SizeBytes, &builtin, &image.InitSystem,
		&image.NetConfigFormat, &image.Origin, &image.LibraryFile,
		&image.UploadedBy, &uploaded, &created, &updated); err != nil {
		return Image{}, err
	}

	image.IsBuiltin = builtin == 1
	image.UploadedAt = parseTimePtr(uploaded)
	image.CreatedAt = parseTime(created)
	image.UpdatedAt = parseTime(updated)
	return image, nil
}

// parseTime reads a stored timestamp, tolerating the two forms SQLite produces:
// RFC3339 from Go writes, and "YYYY-MM-DD HH:MM:SS" from datetime('now') in the
// migration's seed rows.
func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed
	}
	if parsed, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
		return parsed
	}
	return time.Time{}
}

func parseTimePtr(value string) *time.Time {
	parsed := parseTime(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func formatTimePtr(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// nullIfEmpty writes SQL NULL rather than an empty string, so a COALESCE on
// read gives the same answer whichever path wrote the row.
func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullIfZero(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// isUniqueViolation reports whether an error is a UNIQUE constraint failure.
//
// The plugin DB handle returns the driver's error text rather than a typed
// error, so this matches on the message. It is used only to turn a duplicate
// name into a clear "that name is taken" instead of a generic failure.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "unique index")
}
