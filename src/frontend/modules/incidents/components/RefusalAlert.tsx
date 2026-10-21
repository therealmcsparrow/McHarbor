// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSettings } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { reasonOf } from '@resources/hooks/useIncidents';
import { NarrationSettingsDialog } from './NarrationSettingsDialog';

/**
 * Renders a failed narrate, summarise or explain call. `envId` must be the environment the
 * refused subject belongs to, not the globally selected one: opt-in is stored per environment.
 */
export function RefusalAlert({ error, envId }: { error: Error; envId: string | null | undefined }) {
  const { t } = useTranslation('incidents');
  const [settingsOpen, setSettingsOpen] = useState(false);
  const reason = reasonOf(error);

  return (
    <>
      <Alert variant="destructive">
        <span>{reason ? t(`refusal.${reason}`) : error.message}</span>
        {reason === 'not_opted_in' && (
          <Button
            variant="link"
            size="sm"
            className="ml-2 h-auto p-0"
            permission={PERM.INCIDENTS_VIEW}
            disabled={!envId}
            onClick={() => setSettingsOpen(true)}
          >
            <IconSettings className="h-4 w-4" /> {t('settings.open')}
          </Button>
        )}
      </Alert>
      {settingsOpen && envId && <NarrationSettingsDialog envId={envId} onOpenChange={setSettingsOpen} />}
    </>
  );
}
