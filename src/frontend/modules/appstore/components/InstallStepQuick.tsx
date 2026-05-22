// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconRocket, IconChevronRight } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';

interface EnvironmentOption {
  id: string;
  name: string;
}

interface InstallStepQuickProps {
  name: string;
  onNameChange: (value: string) => void;
  slug: string;
  selectedEnvId: string;
  onEnvChange: (value: string) => void;
  dockerEnvs: EnvironmentOption[];
  onInstallDefaults: () => void;
  onCustomize: () => void;
}

export function InstallStepQuick({
  name,
  onNameChange,
  slug,
  selectedEnvId,
  onEnvChange,
  dockerEnvs,
  onInstallDefaults,
  onCustomize,
}: InstallStepQuickProps) {
  const { t } = useTranslation('common');

  return (
    <div className="space-y-4">
      <div>
        <label className="mb-1 block text-sm font-medium text-foreground">
          {t('appStore.stackNameLabel')}
        </label>
        <input
          type="text"
          value={name}
          onChange={(e) => onNameChange(e.target.value)}
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
          placeholder={slug}
        />
      </div>
      <div>
        <label className="mb-1 block text-sm font-medium text-foreground">
          {t('appStore.environmentLabel')}
        </label>
        <Select
          value={selectedEnvId}
          onChange={onEnvChange}
          options={dockerEnvs.map((env) => ({ value: env.id, label: env.name }))}
        />
      </div>
      <div className="flex gap-2">
        <Button
          className="flex-1 gap-1"
          onClick={onInstallDefaults}
          disabled={!name.trim()}
        >
          <IconRocket className="size-4" />
          {t('appStore.installWithDefaults')}
        </Button>
        <Button
          variant="outline"
          className="gap-1"
          onClick={onCustomize}
        >
          {t('appStore.customize')}
          <IconChevronRight className="size-4" />
        </Button>
      </div>
    </div>
  );
}

