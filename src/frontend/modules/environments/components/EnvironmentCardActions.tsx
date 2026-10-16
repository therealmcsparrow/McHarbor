// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconDownload, IconExternalLink, IconRefresh, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { CardFooter } from '@resources/components/ui/Card';
import { PERM } from '@core/auth/permissionKeys';

type Props = {
  environmentId: string;
  isAgent: boolean;
  onTest: (id: string) => void;
  onInstall: (id: string) => void;
  onRemove: (id: string) => void;
};

export function EnvironmentCardActions({ environmentId, isAgent, onTest, onInstall, onRemove }: Props) {
  const { t } = useTranslation('environments');

  return (
    <CardFooter className="gap-1 px-3 py-2">
      <Button asChild variant="ghost" size="icon-sm" aria-label={t('card.open')}>
        <Link to={`/environments/${environmentId}`}>
          <IconExternalLink className="h-4 w-4" />
        </Link>
      </Button>
      <Button
        permission={PERM.ENVIRONMENTS_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('testConnection')}
        title={t('testConnection')}
        onClick={() => onTest(environmentId)}
      >
        <IconRefresh className="h-4 w-4" />
      </Button>
      {isAgent && (
        <Button
          permission={PERM.ENVIRONMENTS_MANAGE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('installAgent')}
          title={t('installAgent')}
          onClick={() => onInstall(environmentId)}
        >
          <IconDownload className="h-4 w-4" />
        </Button>
      )}
      <div className="flex-1" />
      <Button
        permission={PERM.ENVIRONMENTS_MANAGE}
        variant="ghost"
        size="icon-sm"
        aria-label={t('actions.remove', { ns: 'common' })}
        title={t('actions.remove', { ns: 'common' })}
        onClick={() => onRemove(environmentId)}
      >
        <IconTrash className="h-4 w-4 text-destructive" />
      </Button>
    </CardFooter>
  );
}
