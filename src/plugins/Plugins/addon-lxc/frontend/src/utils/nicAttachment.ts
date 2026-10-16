// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

interface AttachableNetwork {
  mode?: string;
  bridge: string;
  vlanTag?: number;
  uplinkLink?: string;
  linkMode?: string;
}

export interface NICAttachment {
  bridge: string;
  vlanTag?: number;
  attachKind?: string;
  parentLink?: string;
  linkMode?: string;
}

export function nicAttachment(network: AttachableNetwork): NICAttachment {
  if (network.mode === 'macvlan') {
    return {
      bridge: '',
      vlanTag: network.vlanTag,
      attachKind: network.mode,
      parentLink: network.uplinkLink,
      linkMode: network.linkMode,
    };
  }
  return { bridge: network.bridge, vlanTag: network.vlanTag };
}
