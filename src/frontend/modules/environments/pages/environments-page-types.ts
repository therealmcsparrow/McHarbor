// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { InstallTokenResponse } from '../hooks/useEnvironmentActions';

/**
 * The one-shot agent credential the create flow hands to the token dialog.
 * `mode` distinguishes a freshly created environment from re-opening the
 * install instructions for an existing one.
 */
export type AgentTokenData = {
  token: string;
  installScript?: InstallTokenResponse | null;
  mode?: 'created' | 'installInfo';
};
