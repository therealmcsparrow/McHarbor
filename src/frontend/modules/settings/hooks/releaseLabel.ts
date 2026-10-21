// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Names one artifact of a release, `version+build`, so a rebuild of the same
 * version reads, and is dismissed, as a different release. Must match
 * `releaseLabel` in modules/updates/release_build.go, which the server stores
 * as the dismissed and notified release.
 */
export function releaseLabel(version: string, build?: string): string {
  return build && /^\d{12}$/.test(build) ? `${version}+${build}` : version;
}
