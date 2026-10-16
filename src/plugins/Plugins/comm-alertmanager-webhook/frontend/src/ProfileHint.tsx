// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Alert } from '@resources/components/ui/Alert';
import type { Profile, Translate } from './options';

export function ProfileHint({ profile, t }: { profile: Profile; t: Translate }) {
  return <Alert variant="info">{t(`profiles.${profile}.hint`)}</Alert>;
}
