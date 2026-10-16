// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useRunbookCatalog } from '@resources/hooks/useRunbooks';
import { HOST_ATTACH_KINDS, type AttachKind } from '@resources/types/runbooks';

export type AttachLabel = { label: string; orphan: boolean };

/**
 * Titles a runbook's attach kind. A host kind reads from the host's own `attach.*` keys; a
 * plugin's kind from the label the plugin declared, in its own namespace. A kind neither the host
 * nor a loaded plugin declares is an orphan of an uninstalled provider, and says so. While the
 * catalog is still loading the raw kind stands in, so a plugin's kind never flashes as an orphan.
 */
export function useAttachLabel(): (kind: AttachKind) => AttachLabel {
  const { t } = useTranslation('runbooks');
  const catalog = useRunbookCatalog();
  const declared = catalog.data?.attachKinds;
  return useCallback(
    (kind) => {
      if ((HOST_ATTACH_KINDS as readonly string[]).includes(kind)) return { label: t(`attach.${kind}`), orphan: false };
      const match = declared?.find((entry) => entry.kind === kind);
      if (match) return { label: t(match.labelKey, { ns: match.labelNs }), orphan: false };
      if (!declared) return { label: kind, orphan: false };
      return { label: t('attach.orphan', { kind }), orphan: true };
    },
    [declared, t],
  );
}
