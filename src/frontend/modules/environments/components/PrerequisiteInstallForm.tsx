// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDownload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Spinner } from '@resources/components/ui/Spinner';
import type { PrerequisiteReport } from '../hooks/useEnvironmentActions';

type PrerequisiteInstallFormProps = {
  installable: PrerequisiteReport['items'];
  selected: string[];
  installing: boolean;
  disabled: boolean;
  onToggle: (key: string, on: boolean) => void;
  onInstall: () => void;
};

export function PrerequisiteInstallForm({
  installable,
  selected,
  installing,
  disabled,
  onToggle,
  onInstall,
}: PrerequisiteInstallFormProps) {
  const { t } = useTranslation('environments');

  return (
    <div className="mt-3 border-t border-border pt-3">
      <p className="text-xs font-medium text-foreground">{t('create.prereq.installTitle')}</p>
      <p className="mt-0.5 text-xs text-muted-foreground">{t('create.prereq.installBody')}</p>
      <div className="mt-2 space-y-1.5">
        {installable.map((item) => (
          <label key={item.key} className="flex items-center gap-2 text-xs text-foreground">
            <Checkbox
              checked={selected.includes(item.key)}
              onCheckedChange={(next) => onToggle(item.key, next === true)}
              aria-label={item.label}
              disabled={installing}
            />
            {item.label}
          </label>
        ))}
      </div>
      <div className="mt-3 flex items-center gap-2">
        <Button
          size="sm"
          permission={PERM.ENVIRONMENTS_MANAGE}
          disabled={disabled || selected.length === 0}
          onClick={onInstall}
        >
          {installing ? (
            <Spinner className="mr-1.5 h-4 w-4" />
          ) : (
            <IconDownload className="mr-1.5 h-4 w-4" aria-hidden="true" />
          )}
          {installing ? t('create.prereq.installing') : t('create.prereq.install')}
        </Button>
        <span className="text-xs text-muted-foreground">{t('create.prereq.escalationNote')}</span>
      </div>
    </div>
  );
}
