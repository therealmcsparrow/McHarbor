// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { SelectionModeToggle } from './SelectionModeToggle';
import { ContainerPathTree } from './ContainerPathTree';
import { ConfigItemTree } from './ConfigItemTree';
import { patchForSelection, selectionOf } from './backupSelectionHelpers';
import type {
  BackupSelectionMode,
  ConfigSection,
  ContainerBackupInput,
  ContainerBackupOption,
} from '@resources/hooks/useContainerBackups';

const CHOOSE_MODE: BackupSelectionMode = 'choose';

type BackupSelectionOptionRowProps = {
  value: ContainerBackupInput;
  options: ContainerBackupOption[];
  option: ContainerBackupOption;
  containerId?: string;
  configSections: ConfigSection[];
  containerInspect?: Record<string, unknown>;
  onChange: (patch: Partial<ContainerBackupInput>) => void;
};

export function BackupSelectionOptionRow({
  value,
  options,
  option,
  containerId,
  configSections,
  containerInspect,
  onChange,
}: BackupSelectionOptionRowProps) {
  const { t } = useTranslation('containers');
  const selection = selectionOf(value, option);
  // "choose" needs something to browse. Image and logs are
  // all-or-nothing by nature; the others need a container id
  // for the live picker, so without one the middle mode is
  // hidden rather than offered and left broken.
  const allowChoose = Boolean(option.partial) && Boolean(containerId);
  const paths = selection.paths ?? [];

  function setMode(mode: BackupSelectionMode) {
    onChange(
      patchForSelection(value, options, option.key, {
        mode,
        paths: mode === 'choose' ? paths : undefined,
      }),
    );
  }

  function setPaths(next: string[]) {
    onChange(
      patchForSelection(value, options, option.key, {
        mode: CHOOSE_MODE,
        paths: next,
      }),
    );
  }

  return (
    <div key={option.key} className="mc-glass space-y-3 rounded-lg p-4">
      <div className="flex items-start justify-between gap-3">
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-medium text-foreground">{option.label}</span>
          <span className="block text-xs text-muted-foreground">{option.description}</span>
        </span>
        <SelectionModeToggle value={selection.mode} onChange={setMode} allowChoose={allowChoose} label={option.label} />
      </div>

      {selection.mode === 'choose' && option.type === 'config' && (
        <ConfigItemTree sections={configSections} inspect={containerInspect} selected={paths} onChange={setPaths} />
      )}
      {selection.mode === 'choose' && option.type !== 'config' && containerId && (
        <ContainerPathTree
          containerId={containerId}
          rootPath={option.key.startsWith('mount:') ? option.key.slice('mount:'.length) : '/'}
          selected={paths}
          onChange={setPaths}
        />
      )}
      {selection.mode === 'choose' && paths.length === 0 && (
        <p className="text-xs text-destructive">{t('backups.chooseNothingPicked')}</p>
      )}
    </div>
  );
}
