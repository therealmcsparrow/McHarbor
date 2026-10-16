// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type ImportState = 'uploaded' | 'inspecting' | 'inspected' | 'converting' | 'ready' | 'failed';

export interface ImportDisk {
  index: number;
  sourceFile: string;
  sourceFormat: string;
  virtualSize: number;
  actualSize?: number;
  volumePath?: string;
}

export interface ImportInspection {
  kind: 'disk' | 'ova';
  disks?: ImportDisk[];
  name?: string;
  vcpus?: number;
  memoryMb?: number;
  firmware?: string;
  guestOsFamily?: string;
  guestOsVariant?: string;
  nicCount?: number;
  warnings?: string[];
}

export interface VMImport {
  id: string;
  fileName: string;
  sourceKind: string;
  sizeBytes: number;
  state: ImportState;
  progress: number;
  lastError?: string;
  environmentId?: string;
  hostPath?: string;
  inspection?: ImportInspection;
  vmId?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ImportListView {
  imports: VMImport[];
  formats: Record<string, string>;
}

export interface ImportConvertInput {
  environmentId: string;
  name: string;
  poolId: string;
  poolBackend: string;
  poolTarget: string;
  networkId: string;
  bridge: string;
  vlanTag?: number;
  attachKind?: string;
  parentLink?: string;
  linkMode?: string;
  vcpus?: number;
  memoryMb?: number;
  firmware?: string;
  guestOsFamily?: string;
  secureBoot?: boolean;
  tpmEnabled?: boolean;
  autostart?: boolean;
}
