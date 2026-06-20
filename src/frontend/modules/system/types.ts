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
