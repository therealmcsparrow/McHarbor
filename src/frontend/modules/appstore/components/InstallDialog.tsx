// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState, useEffect } from 'react';
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

interface InstallDialogProps {
  app: AppTemplate | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type EnvEntry = {
  key: string;
  value: string;
};

function createDefaultNetworkSettings(): AppNetworkSettings {
  return {
    mode: 'default',
    name: '',
    aliases: [],
    ipv4Address: '',
    ipv6Address: '',
    macAddress: '',
  };
}

function entriesToEnvVars(entries: EnvEntry[]): Record<string, string> {
  return entries.reduce<Record<string, string>>((acc, entry) => {
    const key = entry.key.trim();
    if (key) {
      acc[key] = entry.value;
    }
    return acc;
  }, {});
}

function createDefaultEnvEntries(app: AppTemplate, timezone?: string): EnvEntry[] {
  const defaults = new Map<string, string>();
  app.envVars.forEach((ev) => {
    defaults.set(ev.key, ev.default);
  });
  defaults.set('TZ', timezone?.trim() || 'UTC');
  return Array.from(defaults, ([key, value]) => ({ key, value }));
}

export function InstallDialog({ app, open, onOpenChange }: InstallDialogProps) {
  const { t } = useTranslation('common');
  const [step, setStep] = useState(0);
  const [name, setName] = useState('');
  const [ports, setPorts] = useState<PortMapping[]>([]);
  const [volumes, setVolumes] = useState<VolumeMount[]>([]);
  const [network, setNetwork] = useState<AppNetworkSettings>(createDefaultNetworkSettings);
  const [envEntries, setEnvEntries] = useState<EnvEntry[]>([]);
  const [selectedEnvId, setSelectedEnvId] = useState('');
  const currentEnvId = useEnvironmentStore((s) => s.currentId);
  const storedEnvironments = useEnvironmentStore((s) => s.environments);
  const environmentsQuery = useEnvironmentList();
  const environments = environmentsQuery.data ?? storedEnvironments;
  const dockerEnvs = useMemo(
    () => environments.filter((e) => e.orchestratorType === 'docker'),
    [environments],
  );
  const selectedEnvironment = useMemo(
    () => dockerEnvs.find((env) => env.id === selectedEnvId),
    [dockerEnvs, selectedEnvId],
  );
  const envVars = useMemo(() => entriesToEnvVars(envEntries), [envEntries]);
  const { installing, progress, logs, startInstall, abort, reset } = useStreamInstall();

  useEffect(() => {
    if (open && app) {
      const nextSelectedEnvId = dockerEnvs.some((env) => env.id === currentEnvId) ? currentEnvId : dockerEnvs[0]?.id ?? '';
      const nextSelectedEnv = dockerEnvs.find((env) => env.id === nextSelectedEnvId);
      setStep(0);
      setName(app.slug);
      setSelectedEnvId(nextSelectedEnvId);
      setPorts(app.ports.map((p) => ({ ...p })));
      setVolumes(app.volumes.map((v) => ({ ...v })));
      setNetwork(createDefaultNetworkSettings());
      setEnvEntries(createDefaultEnvEntries(app, nextSelectedEnv?.timezone));
      reset();
    }
  }, [open, app, currentEnvId, dockerEnvs, reset]);

  useEffect(() => {
    if (!open || !app || dockerEnvs.length === 0) return;
    if (dockerEnvs.some((env) => env.id === selectedEnvId)) return;
    setSelectedEnvId(dockerEnvs.some((env) => env.id === currentEnvId) ? currentEnvId : dockerEnvs[0]?.id ?? '');
  }, [open, app, currentEnvId, dockerEnvs, selectedEnvId]);

  const handleClose = (isOpen: boolean) => {
    if (!isOpen) abort();
    onOpenChange(isOpen);
  };

  if (!app) return null;

  const handleInstall = (customized: boolean) => {
    const payload = customized
      ? { slug: app.slug, name, environmentId: selectedEnvId, ports, volumes, network, envVars }
      : { slug: app.slug, name, environmentId: selectedEnvId, envVars };
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

  const stepTitles = [t('appStore.stepQuickInstall'), t('appStore.stepPorts'), t('appStore.stepVolumes'), t('appStore.stepNetwork'), t('appStore.stepEnvironment'), t('appStore.stepReview')];
  const totalSteps = 6;

  return (
    <Dialog open={open} onOpenChange={handleClose}>
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
              name={name}
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
            <InstallStepNetwork
              network={network}
              selectedEnvId={selectedEnvId}
              onNetworkChange={setNetwork}
            />
          )}
          {!installing && step === 4 && (
            <InstallStepEnvVars
              envVarDefs={app.envVars}
              envEntries={envEntries}
              timezone={selectedEnvironment?.timezone}
              onEnvEntriesChange={setEnvEntries}
            />
          )}
          {!installing && step === 5 && (
            <InstallStepReview
              name={name}
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
              <Button size="sm" className="gap-1" onClick={() => handleInstall(true)} disabled={!name.trim()}>
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

