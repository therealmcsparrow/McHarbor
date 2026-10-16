// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronRight, IconChevronLeft, IconRocket } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import type { AppNetworkSettings, AppTemplate, PortMapping, VolumeMount } from '../types';
import { useStreamInstall } from '../hooks/useStreamInstall';
import { InstallProgress } from './InstallProgress';
import { InstallStepQuick } from './InstallStepQuick';
import { InstallStepPorts } from './InstallStepPorts';
import { InstallStepVolumes } from './InstallStepVolumes';
import { InstallStepNetwork } from './InstallStepNetwork';
import { InstallStepEnvVars } from './InstallStepEnvVars';
import { InstallStepReview } from './InstallStepReview';
import {
  createDefaultEnvEntries,
  createDefaultNetworkSettings,
  entriesToEnvVars,
  type EnvEntry,
} from './installDialogState';
import { PERM } from '@core/auth/permissionKeys';

interface InstallDialogProps {
  app: AppTemplate | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function InstallDialog({ app, open: openState, onOpenChange }: InstallDialogProps) {
  const { t } = useTranslation('common');
  const [step, setStep] = useState(0);
  const [itemName, setName] = useState('');
  const [ports, setPorts] = useState<PortMapping[]>([]);
  const [volumes, setVolumes] = useState<VolumeMount[]>([]);
  const [network, setNetwork] = useState<AppNetworkSettings>(createDefaultNetworkSettings);
  const [envEntries, setEnvEntries] = useState<EnvEntry[]>([]);
  // Off by default: file mode only works with images that honour the
  // docker-library `_FILE` convention, and the store carries plenty that
  // do not. The toggle on the environment step explains the trade-off.
  const [secretFileMode, setSecretFileMode] = useState(false);
  const [pickedEnvId, setSelectedEnvId] = useState('');
  const currentEnvId = useEnvironmentStore((s) => s.currentId);
  const storedEnvironments = useEnvironmentStore((s) => s.environments);
  const environmentsQuery = useEnvironmentList();
  const environments = environmentsQuery.data ?? storedEnvironments;
  const dockerEnvs = useMemo(() => environments, [environments]);
  const selectedEnvId =
    dockerEnvs.length === 0 || dockerEnvs.some((env) => env.id === pickedEnvId)
      ? pickedEnvId
      : dockerEnvs.some((env) => env.id === currentEnvId)
        ? currentEnvId
        : (dockerEnvs[0]?.id ?? '');
  const selectedEnvironment = useMemo(
    () => dockerEnvs.find((env) => env.id === selectedEnvId),
    [dockerEnvs, selectedEnvId],
  );
  const envVars = useMemo(() => entriesToEnvVars(envEntries), [envEntries]);
  const { installing, progress, logs, startInstall, abort, reset } = useStreamInstall();

  // The environment list refetches every 30 s with a new array. Initialising on every change
  // reset the dialog to its first step mid-install (a long image pull outlives a refetch) and
  // threw away the operator's customisations, so it runs once per opening instead.
  const initializedFor = useRef<string | null>(null);
  useEffect(() => {
    if (!openState || !app) {
      initializedFor.current = null;
      return;
    }
    if (initializedFor.current === app.slug) return;
    initializedFor.current = app.slug;
    const nextSelectedEnvId = dockerEnvs.some((env) => env.id === currentEnvId)
      ? currentEnvId
      : (dockerEnvs[0]?.id ?? '');
    const nextSelectedEnv = dockerEnvs.find((env) => env.id === nextSelectedEnvId);
    setStep(0);
    setName(app.slug);
    setSelectedEnvId(nextSelectedEnvId);
    setPorts(app.ports.map((p) => ({ ...p })));
    setVolumes(app.volumes.map((v) => ({ ...v })));
    setNetwork(createDefaultNetworkSettings());
    setEnvEntries(createDefaultEnvEntries(app, nextSelectedEnv?.timezone));
    reset();
  }, [openState, app, currentEnvId, dockerEnvs, reset]);

  const handleClose = (isOpen: boolean) => {
    if (!isOpen) abort();
    onOpenChange(isOpen);
  };

  if (!app) return null;

  const handleInstall = (customized: boolean) => {
    const payload = customized
      ? {
          slug: app.slug,
          name: itemName,
          environmentId: selectedEnvId,
          ports,
          volumes,
          network,
          envVars,
          secretFileMode,
        }
      : { slug: app.slug, name: itemName, environmentId: selectedEnvId, envVars, secretFileMode };
    startInstall(payload);
  };

  const handleEnvChange = (value: string) => {
    const timezone = dockerEnvs.find((env) => env.id === value)?.timezone?.trim() || 'UTC';
    setSelectedEnvId(value);
    setNetwork(createDefaultNetworkSettings());
    setEnvEntries((entries) => {
      if (entries.some((entry) => entry.key.trim() === 'TZ')) {
        return entries.map((entry) => (entry.key.trim() === 'TZ' ? { ...entry, value: timezone } : entry));
      }
      return [...entries, { key: 'TZ', value: timezone }];
    });
  };

  const updatePort = (index: number, field: 'host' | 'container', value: number) => {
    setPorts((prev) => prev.map((p, i) => (i === index ? { ...p, [field]: value } : p)));
  };

  const updateVolume = (index: number, field: 'host' | 'container', value: string) => {
    setVolumes((prev) => prev.map((v, i) => (i === index ? { ...v, [field]: value } : v)));
  };

  const stepTitles = [
    t('appStore.stepQuickInstall'),
    t('appStore.stepPorts'),
    t('appStore.stepVolumes'),
    t('appStore.stepNetwork'),
    t('appStore.stepEnvironment'),
    t('appStore.stepReview'),
  ];
  const totalSteps = 6;

  return (
    <Dialog open={openState} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('appStore.installTitle', { name: app.name })}</DialogTitle>
          <DialogDescription>
            {installing
              ? t('appStore.installing')
              : step === 0
                ? t('appStore.installDefaultsDesc')
                : t('appStore.installStepLabel', { step, total: totalSteps - 1, title: stepTitles[step] })}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-[200px]">
          {installing && <InstallProgress progress={progress} logs={logs} onClose={() => onOpenChange(false)} />}
          {!installing && step === 0 && (
            <InstallStepQuick
              name={itemName}
              onNameChange={setName}
              slug={app.slug}
              selectedEnvId={selectedEnvId}
              onEnvChange={handleEnvChange}
              dockerEnvs={dockerEnvs}
              onInstallDefaults={() => handleInstall(false)}
              onCustomize={() => setStep(1)}
            />
          )}
          {!installing && step === 1 && <InstallStepPorts ports={ports} onPortChange={updatePort} />}
          {!installing && step === 2 && <InstallStepVolumes volumes={volumes} onVolumeChange={updateVolume} />}
          {!installing && step === 3 && (
            <InstallStepNetwork network={network} selectedEnvId={selectedEnvId} onNetworkChange={setNetwork} />
          )}
          {!installing && step === 4 && (
            <InstallStepEnvVars
              envVarDefs={app.envVars}
              envEntries={envEntries}
              timezone={selectedEnvironment?.timezone}
              onEnvEntriesChange={setEnvEntries}
              secretFileMode={secretFileMode}
              onSecretFileModeChange={setSecretFileMode}
            />
          )}
          {!installing && step === 5 && (
            <InstallStepReview
              name={itemName}
              image={app.image}
              selectedEnvId={selectedEnvId}
              dockerEnvs={dockerEnvs}
              ports={ports}
              volumes={volumes}
              network={network}
              envVars={envVars}
            />
          )}
        </div>

        {!installing && step > 0 && (
          <DialogFooter className="flex items-center justify-between">
            <Button variant="outline" size="sm" className="gap-1" onClick={() => setStep((s) => s - 1)}>
              <IconChevronLeft className="size-4" />
              {t('appStore.back')}
            </Button>
            {step < 5 ? (
              <Button size="sm" className="gap-1" onClick={() => setStep((s) => s + 1)}>
                {t('appStore.next')}
                <IconChevronRight className="size-4" />
              </Button>
            ) : (
              <Button
                permission={PERM.STORE_APPS_MANAGE}
                size="sm"
                className="gap-1"
                onClick={() => handleInstall(true)}
                disabled={!itemName.trim()}
              >
                <IconRocket className="size-4" />
                {t('actions.deploy')}
              </Button>
            )}
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
