// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/** Must stay in sync with `DisplayProfile` in backend/display_profiles.go. */
export type DisplayProfile = {
  id: string;
  name: string;
  description?: string;
  builtin: boolean;
  imageCompression: string;
  jpegWanCompression: string;
  zlibGlzWanCompression: string;
  streamingVideo: string;
  playbackCompression: string;
  agentMouse: string;
  vmCount: number;
  createdAt: string;
  updatedAt: string;
};

export type DisplayProfileOption =
  | 'imageCompression'
  | 'jpegWanCompression'
  | 'zlibGlzWanCompression'
  | 'streamingVideo'
  | 'playbackCompression'
  | 'agentMouse';

export type DisplayProfileValue =
  | 'auto_glz'
  | 'auto_lz'
  | 'quic'
  | 'glz'
  | 'lz'
  | 'off'
  | 'auto'
  | 'never'
  | 'always'
  | 'all'
  | 'filter'
  | 'on';

export type DisplayProfileDraft = Pick<DisplayProfile, 'name' | DisplayProfileOption> & {
  description: string;
};

/** QEMU's closed value set per option, mirroring spiceOptionSets on the server. */
export const DISPLAY_PROFILE_OPTIONS: readonly {
  key: DisplayProfileOption;
  values: readonly DisplayProfileValue[];
}[] = [
  { key: 'imageCompression', values: ['off', 'lz', 'auto_lz', 'quic', 'auto_glz', 'glz'] },
  { key: 'streamingVideo', values: ['filter', 'all', 'off'] },
  { key: 'jpegWanCompression', values: ['auto', 'never', 'always'] },
  { key: 'zlibGlzWanCompression', values: ['never', 'auto', 'always'] },
  { key: 'playbackCompression', values: ['on', 'off'] },
  { key: 'agentMouse', values: ['on', 'off'] },
];

export const PROFILE_FAST_LAN = 'builtin-fast-lan';
export const PROFILE_BALANCED = 'builtin-balanced';
export const PROFILE_LOW_BANDWIDTH = 'builtin-low-bandwidth';

/** A new profile starts from Fast LAN, as the server does for an unset option. */
export const NEW_PROFILE_DRAFT: DisplayProfileDraft = {
  name: '',
  description: '',
  imageCompression: 'off',
  jpegWanCompression: 'never',
  zlibGlzWanCompression: 'never',
  streamingVideo: 'filter',
  playbackCompression: 'on',
  agentMouse: 'on',
};
