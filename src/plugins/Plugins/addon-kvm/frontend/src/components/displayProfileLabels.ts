// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import {
  PROFILE_BALANCED,
  PROFILE_FAST_LAN,
  PROFILE_LOW_BANDWIDTH,
  type DisplayProfile,
  type DisplayProfileOption,
  type DisplayProfileValue,
} from '../types-display';

type Translate = TFunction<'vm'>;

/** A built-in's name is translated; a custom profile's is what the operator typed. */
export function profileName(t: Translate, profile: Pick<DisplayProfile, 'id' | 'name'>): string {
  switch (profile.id) {
    case PROFILE_FAST_LAN:
      return t('displayProfiles.builtin.fastLan.name');
    case PROFILE_BALANCED:
      return t('displayProfiles.builtin.balanced.name');
    case PROFILE_LOW_BANDWIDTH:
      return t('displayProfiles.builtin.lowBandwidth.name');
    default:
      return profile.name;
  }
}

export function profileDescription(t: Translate, profile: DisplayProfile): string {
  switch (profile.id) {
    case PROFILE_FAST_LAN:
      return t('displayProfiles.builtin.fastLan.description');
    case PROFILE_BALANCED:
      return t('displayProfiles.builtin.balanced.description');
    case PROFILE_LOW_BANDWIDTH:
      return t('displayProfiles.builtin.lowBandwidth.description');
    default:
      return profile.description ?? '';
  }
}

const KNOWN_VALUES: readonly DisplayProfileValue[] = [
  'auto_glz',
  'auto_lz',
  'quic',
  'glz',
  'lz',
  'off',
  'auto',
  'never',
  'always',
  'all',
  'filter',
  'on',
];

function isKnownValue(value: string): value is DisplayProfileValue {
  return KNOWN_VALUES.some((known) => known === value);
}

/** A value outside the closed set (a row written by a newer build) shows raw. */
export function valueLabel(t: Translate, value: string): string {
  return isKnownValue(value) ? t(`displayProfiles.values.${value}`) : value;
}

export function optionLabel(t: Translate, key: DisplayProfileOption): string {
  return t(`displayProfiles.options.${key}.label`);
}

export function optionHint(t: Translate, key: DisplayProfileOption): string {
  return t(`displayProfiles.options.${key}.hint`);
}
