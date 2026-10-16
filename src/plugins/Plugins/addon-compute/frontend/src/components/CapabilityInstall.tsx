// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconDownload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { CapabilityInstallError, useInstallCapability } from '../hooks/useCompute';
import { hostToast } from '../hostToast';
import { COMPUTE_PERM } from '../permissions';
import type { CapabilityItem } from '../types';

type CapabilityInstallProps = {
  environmentId: string;
  item: CapabilityItem;
  label: string;
};

/**
 * Install button for a missing capability item the host's agent can install itself. The agent
 * picks the packages; the dialog shows the command the probe suggested so the operator sees
 * what will run. A failure keeps the dialog open with the host's own words and output.
 */
export function CapabilityInstall({ environmentId, item, label }: CapabilityInstallProps) {
  const { t } = useTranslation('compute');
  const [dialogOpen, setDialogOpen] = useState(false);
  const install = useInstallCapability();
  const failure = install.error instanceof CapabilityInstallError ? install.error : null;

  const runInstall = () => {
    if (install.isPending) return;
    install.mutate(
      { envId: environmentId, item: item.key },
      {
        onSuccess: (result) => {
          setDialogOpen(false);
          hostToast.success(t('capability.installDone', { item: label, packages: (result.packages ?? []).join(', ') }));
        },
      },
    );
  };

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        permission={COMPUTE_PERM.MANAGE}
        onClick={() => {
          install.reset();
          setDialogOpen(true);
        }}
      >
        <IconDownload className="h-4 w-4" aria-hidden="true" />
        {t('capability.installButton')}
      </Button>
      <ConfirmDialog
        open={dialogOpen}
        onOpenChange={(next) => {
          if (!install.isPending) setDialogOpen(next);
        }}
        title={t('capability.installTitle', { item: label })}
        description={t('capability.installDescription', { item: label })}
        confirmLabel={t('capability.installConfirm')}
        variant="caution"
        loading={install.isPending}
        permission={COMPUTE_PERM.MANAGE}
        onConfirm={runInstall}
      >
        {item.remediation && (
          <code className="block overflow-x-auto rounded bg-muted px-2 py-1.5 font-mono text-xs text-foreground">
            {item.remediation}
          </code>
        )}
        {failure && (
          <div role="alert" className="mt-3 space-y-2">
            <p className="text-sm font-medium text-destructive">{t('capability.installFailed')}</p>
            {failure.detail && <p className="text-sm text-foreground">{failure.detail}</p>}
            {failure.output && (
              <pre className="max-h-48 overflow-auto rounded bg-muted p-2 font-mono text-xs whitespace-pre-wrap text-muted-foreground">
                {failure.output}
              </pre>
            )}
          </div>
        )}
      </ConfirmDialog>
    </>
  );
}
