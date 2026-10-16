// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { RunbookParamSpec, RunbookTargetKind } from '@resources/types/runbooks';

/**
 * A target parameter is titled by the kind it selects, in the namespace of the plugin that
 * declared the kind, so a kind the host has never heard of still reads as words. Host kinds and
 * plain parameters keep the host's own `param.*` wording.
 */
export function useParamLabel(targets: RunbookTargetKind[] | undefined): (spec: RunbookParamSpec) => string {
  const { t } = useTranslation('runbooks');
  return (spec) => {
    const declared = spec.type === 'target' ? targets?.find((kind) => kind.kind === spec.target) : undefined;
    return declared ? t(declared.labelKey, { ns: declared.labelNs }) : t(`param.${spec.name}`);
  };
}
