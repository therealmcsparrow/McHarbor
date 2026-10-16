// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { BackupSelectionMode } from '@resources/hooks/useContainerBackups';
import { RestoreFileTree } from './RestoreFileTree';
import { SelectionModeToggle } from './SelectionModeToggle';

type RestoreLayerSectionProps = {
  title: string;
  description?: string;
  toggleLabel: string;
  hintLayer: string;
  mode: BackupSelectionMode;
  onModeChange: (mode: BackupSelectionMode) => void;
  runId: string;
  mountTarget: string;
  kind: 'filesystem' | 'config' | 'mount';
  secretKey: string;
  selected: string[];
  onSelectedChange: (next: string[]) => void;
};

// RestoreLayerSection is one restore layer card: title, per-layer mode toggle
// and either a hint (off / full) or the tree picker (choose).
export function RestoreLayerSection({
  title,
  description,
  toggleLabel,
  hintLayer,
  mode,
  onModeChange,
  runId,
  mountTarget,
  kind,
  secretKey,
  selected,
  onSelectedChange,
}: RestoreLayerSectionProps) {
  const { t } = useTranslation('containers');
  return (
    <section className="space-y-2 rounded-md border border-border p-3">
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-foreground">{title}</p>
          {description && <p className="text-xs text-muted-foreground">{description}</p>}
        </div>
        <SelectionModeToggle value={mode} onChange={onModeChange} label={toggleLabel} />
      </header>
      {mode === 'off' ? (
        <p className="rounded-md border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
          {t('backups.restoreOffHint', { layer: hintLayer })}
        </p>
      ) : mode === 'full' ? (
        <p className="rounded-md border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
          {t('backups.restoreFullHint', { layer: hintLayer })}
        </p>
      ) : (
        <RestoreFileTree
          runId={runId}
          mountTarget={mountTarget}
          secretKey={secretKey}
          selected={selected}
          onChange={onSelectedChange}
          kind={kind}
        />
      )}
    </section>
  );
}
