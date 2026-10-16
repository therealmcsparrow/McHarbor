// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package host contains host-only helpers that are NEVER imported
// by plugins — label registry, manifest schema validation, trusted
// public keys, the `NewDockerPoolAccessor` constructor, and the
// `*AdapterSource` interfaces that bridge host modules to the
// plugin runtime.
//
// Why split: the previous monolithic `core/plugin` package mixed
// host-only helpers (LabelRegistry, ValidateManifestSchema, etc.)
// with plugin-imported types. The host's reach set grew because the
// loader referenced all of them; the plugin's reach set didn't (the
// plugin prunes them). That asymmetry produced the per-package ABI
// hash mismatch.
//
// `host` is imported only by host modules. Plugins don't see it, so
// the plugin linker never compares it against anything — no hash
// can mismatch. This is the cleanup half of the shim refactor: the
// shared packages (`contract`, `specs/*`, `services`) have
// identical reach sets between host and plugin, and the host-only
// code lives somewhere the plugin never imports.
//
// Imported by:
//   - `core/pluginruntime/loader.go` (the host's loader)
//   - host modules that validate manifests or call adapter sources
//     (`modules/appstore/plugins/`,
//     `modules/storage_locations/`,
//     `modules/communications/`,
//     `modules/container_backups/`)
package host
