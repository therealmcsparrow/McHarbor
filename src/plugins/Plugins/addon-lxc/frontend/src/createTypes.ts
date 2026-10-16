// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { AddressMode, InitSystem, Limits, MountKind } from './types';

export interface CreateContainerInput {
  environmentId: string;
  name: string;
  description?: string;

  /** Exactly one of these. */
  imageId?: string;
  templateId?: string;

  poolId: string;
  poolBackend: string;
  poolTarget: string;

  initSystem?: InitSystem;

  /** Both need ct.privileged. */
  privileged?: boolean;
  nesting?: boolean;

  limits: Limits;

  nics?: CreateNICInput[];
  mounts?: CreateMountInput[];

  hostname?: string;
  timezone?: string;
  sshKeys?: string[];
  /** Hashed on the server before it is stored, and never written to disk. */
  password?: string;

  autostart?: boolean;
  startAfterCreate?: boolean;
}

export interface CreateNICInput {
  networkId: string;
  bridge: string;
  vlanTag?: number;
  attachKind?: string;
  parentLink?: string;
  linkMode?: string;
  guestName?: string;
  addressMode?: AddressMode;
  staticIp?: string;
  staticGw?: string;
  staticIpv6?: string;
  staticGw6?: string;
  staticDns?: string[];
}

export interface CreateMountInput {
  kind: MountKind;
  source: string;
  destination: string;
  readonly?: boolean;
  shiftIds?: boolean;
}
