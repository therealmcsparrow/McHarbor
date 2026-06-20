// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconActivity, IconHeartRateMonitor, IconRefresh } from '@tabler/icons-react';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { useAutohealPreference, useSetAutohealPreference } from '@resources/hooks/useAutohealPreference';

type AutohealCardProps = {
  containerId: string;
};

export function AutohealCard({ containerId }: AutohealCardProps) {
  const { t } = useTranslation('containers');
  const { data: pref, isLoading } = useAutohealPreference(containerId);
  const setPref = useSetAutohealPreference(containerId);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <IconHeartRateMonitor className="h-4 w-4 text-muted-foreground" />
          {t('autoheal.title')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {isLoading || !pref ? (
          <div className="flex h-20 items-center justify-center">
            <Spinner size="md" />
          </div>
        ) : (
          <>
            <div className="flex items-center justify-between">
              <Label htmlFor="autoheal-enabled" className="cursor-pointer">
                <div className="text-sm font-medium">{t('autoheal.enable')}</div>
                <div className="text-xs text-muted-foreground">{t('autoheal.enableDesc')}</div>
              </Label>
              <Switch
                id="autoheal-enabled"
                checked={pref.enabled}
                disabled={setPref.isPending}
                onCheckedChange={(checked) => setPref.mutate(checked)}
              />
            </div>

            {pref.enabled && (
              <div className="space-y-1.5 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
                <div className="flex items-center justify-between">
                  <span className="flex items-center gap-1.5">
                    <IconActivity className="h-3.5 w-3.5" />
                    {t('autoheal.restartCount', { count: pref.restartCount })}
                  </span>
                  <span>
                    {pref.wasEverHealthy ? t('autoheal.wasHealthy') : t('autoheal.neverHealthy')}
                  </span>
                </div>
                {pref.lastHealAt && (
                  <div className="flex items-center justify-between">
                    <span className="flex items-center gap-1.5">
                      <IconRefresh className="h-3.5 w-3.5" />
                      {t('autoheal.lastHeal')}
                    </span>
                    <span className="font-mono text-foreground">{pref.lastHealAt}</span>
                  </div>
                )}
              </div>
            )}

            <p className="text-[11px] leading-snug text-muted-foreground">
              {t('autoheal.footnote')}
            </p>
          </>
        )}
      </CardContent>
    </Card>
  );
}
