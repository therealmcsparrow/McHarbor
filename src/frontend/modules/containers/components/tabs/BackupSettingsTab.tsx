// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { Spinner } from '@resources/components/ui/Spinner';
import { useStorageLocations } from '@resources/hooks/useStorageLocations';
import { useContainer } from '../../hooks/useContainers';
import { fullBackupPatch } from '../BackupSelectionFields';
import { defaultLocalStorage, withPrimaryDestination } from '../plan-utils';
import { BackupSettingsPanel } from './BackupSettingsPanel';
import {
  type ContainerBackupInput,
  type ContainerBackupOption,
  useContainerBackupOptions,
  useContainerBackupRuns,
  useCreateContainerBackupPlan,
  useRunContainerBackup,
} from '@resources/hooks/useContainerBackups';

type BackupSettingsTabProps = {
  containerId: string;
  containerName: string;
};

const DEFAULT_INPUT: ContainerBackupInput = {
  name: '',
  storageLocationId: '',
  storageLocationIds: [],
  includeConfig: true,
  includeLogs: false,
  includeFilesystem: false,
  includeImage: false,
  selectedMounts: [],
  cron: '',
  enabled: true,
  retentionCount: 0,
  retentionDays: 0,
  consistencyMode: 'pause',
  snapshotConfigId: '',
};

function defaultInput(options: ContainerBackupOption[], containerName: string): ContainerBackupInput {
  return {
    ...DEFAULT_INPUT,
    name: `${containerName} backup`,
    includeLogs: options.some((option) => option.type === 'logs' && option.default),
    includeFilesystem: options.some((option) => option.type === 'filesystem' && option.default),
    includeImage: options.some((option) => option.type === 'image' && option.default),
    selectedMounts: options
      .filter((option) => option.key.startsWith('mount:') && option.default)
      .map((option) => option.key.slice('mount:'.length)),
  };
}

export function BackupSettingsTab({ containerId, containerName }: BackupSettingsTabProps) {
  const { data: optionData, isLoading: optionsLoading } = useContainerBackupOptions(containerId);
  const { data: runs = [], isLoading: runsLoading } = useContainerBackupRuns(containerId);
  const { data: storageLocations = [], isLoading: storageLocationsLoading } = useStorageLocations();
  // The live inspect backs the per-item config picker. It is already
  // cached by the detail page, so this is a cache read rather than an
  // extra request in practice.
  const { data: containerInspect } = useContainer(containerId);
  const runAdhoc = useRunContainerBackup(containerId);
  const createPlan = useCreateContainerBackupPlan(containerId);

  // One shared input drives both the ad-hoc run and the schedule. The
  // ad-hoc endpoint decodes into RunBackupInput and simply ignores the
  // cron / retention fields, so there is no need for two copies.
  const [input, setInput] = useState<ContainerBackupInput>(DEFAULT_INPUT);
  const [scheduled, setScheduled] = useState(false);
  const [fullBackup, setFullBackup] = useState(true);

  const options = useMemo(() => optionData?.options ?? [], [optionData?.options]);
  const configSections = useMemo(() => optionData?.configSections ?? [], [optionData?.configSections]);
  const defaultLocation = useMemo(() => defaultLocalStorage(storageLocations), [storageLocations]);
  const defaultLocationId = defaultLocation?.id;
  const adhocBackupRunning = useMemo(
    () => runs.some((run) => run.operation === 'backup' && run.status === 'running' && !run.planId),
    [runs],
  );

  // Seeds the form once the option list arrives, and only then. Two
  // defaults are applied here and nowhere else, which is what lets the
  // user override either without the form fighting back:
  //   - the full selection, because the Full backup toggle defaults on
  //     (the effect deliberately does not depend on `fullBackup`);
  //   - the local storage location, because a backup should land
  //     somewhere by default. `updateInput` never re-adds it, so it can
  //     be switched off in favour of a remote-only destination.
  const [seeded, setSeeded] = useState<{
    containerName: string;
    defaultLocationId: string | undefined;
    options: unknown;
  }>({
    containerName,
    defaultLocationId,
    options: undefined,
  });
  if (
    options.length > 0 &&
    (seeded.options !== options ||
      seeded.containerName !== containerName ||
      seeded.defaultLocationId !== defaultLocationId)
  ) {
    setSeeded({ containerName, defaultLocationId, options });
    setInput(
      withPrimaryDestination({
        ...defaultInput(options, containerName),
        ...fullBackupPatch(options),
        storageLocationIds: defaultLocationId ? [defaultLocationId] : [],
      }),
    );
  }

  function updateInput(patch: Partial<ContainerBackupInput>) {
    setInput((current) => withPrimaryDestination({ ...current, ...patch }));
  }

  // Turning the schedule on seeds a nightly cron so "Add schedule" is
  // immediately valid; turning it back off leaves the value alone in
  // case the user flips it on again.
  function toggleScheduled(next: boolean) {
    setScheduled(next);
    if (next && !input.cron?.trim()) {
      updateInput({ cron: '0 3 * * *', enabled: input.enabled ?? true });
    }
  }

  if (optionsLoading || runsLoading || storageLocationsLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  return (
    <BackupSettingsPanel
      value={input}
      options={options}
      storageLocations={storageLocations}
      containerId={containerId}
      configSections={configSections}
      containerInspect={containerInspect as Record<string, unknown> | undefined}
      fullBackup={fullBackup}
      onFullBackupChange={setFullBackup}
      scheduled={scheduled}
      onScheduledChange={toggleScheduled}
      onChange={updateInput}
      onRunNow={() => runAdhoc.mutate(input)}
      onSaveSchedule={() => createPlan.mutate(input)}
      runPending={runAdhoc.isPending}
      runDisabled={adhocBackupRunning}
      savePending={createPlan.isPending}
    />
  );
}
