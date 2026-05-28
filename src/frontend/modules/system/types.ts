// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type SystemDependency = {
  name: string;
  version: string;
};

export type SystemInfo = {
  version: string;
  goVersion: string;
  platform: string;
  dependencies: SystemDependency[];
};

export type OSLogSource = "system" | "kernel" | "auth" | "docker";

export type OSLogResult = {
  source: OSLogSource;
  tail: number;
  lines: string[];
  notices: string[];
  fetchedAt: string;
};

export type OSUpdateCheckResult = {
  manager: string;
  available: boolean;
  updates: string[];
  output: string;
  checkedAt: string;
};

export type OSUpdateApplyResult = {
  manager: string;
  exitCode: number;
  success: boolean;
  output: string;
  ranAt: string;
};
