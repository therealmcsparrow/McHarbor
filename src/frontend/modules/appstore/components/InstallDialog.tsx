// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
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
import type { AppTemplate, PortMapping, VolumeMount } from '../types';
import { useStreamInstall } from '../hooks/useStreamInstall';
import { InstallProgress } from './InstallProgress';
import { InstallStepQuick } from './InstallStepQuick';
import { InstallStepPorts } from './InstallStepPorts';
import { InstallStepVolumes } from './InstallStepVolumes';
import { InstallStepEnvVars } from './InstallStepEnvVars';
import { InstallStepReview } from './InstallStepReview';

interface InstallDialogProps {
  app: AppTemplate | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function InstallDialog({ app, open, onOpenChange }: InstallDialogProps) {
  const { t } = useTranslation('common');
  const [step, setStep] = useState(0);
  const [name, setName] = useState('');
  const [ports, setPorts] = useState<PortMapping[]>([]);
  const [volumes, setVolumes] = useState<VolumeMount[]>([]);
  const [envVars, setEnvVars] = useState<Record<string, string>>({});
  const [selectedEnvId, setSelectedEnvId] = useState('');
  const currentEnvId = useEnvironmentStore((s) => s.currentId);
  const environments = useEnvironmentStore((s) => s.environments);
  const dockerEnvs = environments.filter((e) => e.orchestratorType === 'docker');
  const { installing, progress, logs, startInstall, abort, reset } = useStreamInstall();

  useEffect(() => {
    if (open && app) {
      setStep(0);
      setName(app.slug);
      setSelectedEnvId(currentEnvId || dockerEnvs[0]?.id || '');
      setPorts(app.ports.map((p) => ({ ...p })));
      setVolumes(app.volumes.map((v) => ({ ...v })));
      const defaults: Record<string, string> = {};
      app.envVars.forEach((ev) => { defaults[ev.key] = ev.default; });
      setEnvVars(defaults);
      reset();
    }
  }, [open, app]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleClose = (isOpen: boolean) => {
    if (!isOpen) abort();
    onOpenChange(isOpen);
  };

  if (!app) return null;

  const handleInstall = (customized: boolean) => {
    const payload = customized
      ? { slug: app.slug, name, environmentId: selectedEnvId, ports, volumes, envVars }
      : { slug: app.slug, name, environmentId: selectedEnvId };
    startInstall(payload);
  };

  const updatePort = (index: number, field: 'host' | 'container', value: number) => {
    setPorts((prev) => prev.map((p, i) => (i === index ? { ...p, [field]: value } : p)));
  };

  const updateVolume = (index: number, field: 'host' | 'container', value: string) => {
    setVolumes((prev) => prev.map((v, i) => (i === index ? { ...v, [field]: value } : v)));
  };

  const stepTitles = [t('appStore.stepQuickInstall'), t('appStore.stepPorts'), t('appStore.stepVolumes'), t('appStore.stepEnvironment'), t('appStore.stepReview')];
  const totalSteps = 5;

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-lg">
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
              onEnvChange={setSelectedEnvId}
              dockerEnvs={dockerEnvs}
              onInstallDefaults={() => handleInstall(false)}
              onCustomize={() => setStep(1)}
            />
          )}
          {!installing && step === 1 && <InstallStepPorts ports={ports} onPortChange={updatePort} />}
          {!installing && step === 2 && <InstallStepVolumes volumes={volumes} onVolumeChange={updateVolume} />}
          {!installing && step === 3 && (
            <InstallStepEnvVars
              envVarDefs={app.envVars}
              envVars={envVars}
              onEnvVarChange={(key, value) => setEnvVars((prev) => ({ ...prev, [key]: value }))}
            />
          )}
          {!installing && step === 4 && (
            <InstallStepReview
              name={name}
              image={app.image}
              selectedEnvId={selectedEnvId}
              dockerEnvs={dockerEnvs}
              ports={ports}
              volumes={volumes}
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
            {step < 4 ? (
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

