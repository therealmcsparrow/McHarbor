// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { lazy, Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlugX, IconTerminal } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { useContainers } from '@resources/hooks/useContainers';
import { truncateId } from '@resources/utils/format';
import { USER_OPTIONS, USER_PRESET_DEFAULT, USER_PRESET_OTHER } from '@resources/components/TerminalShellOptions';
import { useApplicableShells } from '@resources/hooks/useContainerShells';
import {
  TERMINAL_FULLSCREEN_CLASS,
  TerminalFullscreenButton,
  useTerminalFullscreen,
} from '@resources/components/TerminalFullscreen';
import { PERM } from '@core/auth/permissionKeys';

const TerminalSession = lazy(() => import('../components/TerminalSession'));

export default function TerminalPage() {
  const { t } = useTranslation('terminal');
  const { data: containers = [] } = useContainers();
  const runningContainers = containers.filter((c) => c.State === 'running');

  const [selectedId, setSelectedId] = useState('');
  const [sessionId, setSessionId] = useState('');
  const sessionActive = sessionId.length > 0;
  const { options: shellOptions } = useApplicableShells(selectedId);
  const [shell, setShell] = useState('');
  const [userPreset, setUserPreset] = useState(USER_PRESET_DEFAULT);
  const [customUser, setCustomUser] = useState('');
  const [workingDir, setWorkingDir] = useState('');
  const { fullscreen, toggle: toggleFullscreen } = useTerminalFullscreen();

  // Resolved user identity passed to the backend. Empty string
  // means "let the container's image USER apply" (Docker only sets
  // the user when the field is non-empty).
  const resolvedUser =
    userPreset === USER_PRESET_OTHER ? customUser.trim() : userPreset === USER_PRESET_DEFAULT ? '' : userPreset;

  const handleConnect = () => {
    if (!selectedId) {
      return;
    }

    setSessionId(selectedId);
  };

  const handleDisconnect = () => {
    setSessionId('');
  };

  return (
    <div className={fullscreen ? TERMINAL_FULLSCREEN_CLASS : 'flex h-full flex-col space-y-4'}>
      {/* The header is the first thing worth trading away for terminal rows. */}
      {!fullscreen && <PageHeader title={t('title')} description={t('description')} headingLeaf />}

      <div className="flex flex-wrap items-center gap-3">
        <Select
          variant="outline"
          value={selectedId}
          onChange={(value) => {
            setSelectedId(value);
            if (sessionId && value !== sessionId) {
              setSessionId('');
            }
          }}
          placeholder={t('selectContainerPlaceholder')}
          options={runningContainers.map((c) => ({
            value: c.Id,
            label: c.Names?.[0]?.replace(/^\//, '') ?? truncateId(c.Id),
          }))}
          className="w-64"
        />
        <div className="w-32">
          <Select
            value={shell}
            onChange={setShell}
            options={shellOptions}
            ariaLabel={t('shellLabel')}
            searchable={false}
            disabled={!selectedId}
          />
        </div>
        <div className="w-44">
          <Select
            value={userPreset}
            onChange={setUserPreset}
            options={USER_OPTIONS}
            ariaLabel={t('userLabel')}
            searchable={false}
            disabled={!selectedId}
          />
        </div>
        {userPreset === USER_PRESET_OTHER && (
          <Input
            value={customUser}
            onChange={(e) => setCustomUser(e.target.value)}
            placeholder={t('userPlaceholder')}
            aria-label={t('userLabel')}
            className="w-44"
            disabled={!selectedId}
          />
        )}
        <Input
          value={workingDir}
          onChange={(e) => setWorkingDir(e.target.value)}
          placeholder={t('workDirPlaceholder')}
          aria-label={t('workDirLabel')}
          className="w-40"
          disabled={!selectedId}
        />
        <Button permission={PERM.TERMINAL_ACCESS} onClick={handleConnect} disabled={!selectedId || sessionActive}>
          <IconTerminal className="h-4 w-4" /> {t('connect')}
        </Button>
        {sessionActive && (
          <Button variant="outline" onClick={handleDisconnect}>
            <IconPlugX className="h-4 w-4" /> {t('disconnect')}
          </Button>
        )}
        <TerminalFullscreenButton fullscreen={fullscreen} onToggle={toggleFullscreen} />
      </div>

      {sessionActive ? (
        <Suspense
          fallback={
            <div className="min-h-[400px] flex-1 rounded-lg border border-border bg-background p-4">
              <div className="h-full min-h-[360px] animate-pulse rounded bg-muted/80" />
            </div>
          }
        >
          <TerminalSession
            containerId={sessionId}
            active={sessionActive}
            onDisconnect={handleDisconnect}
            shell={shell}
            user={resolvedUser}
            workingDir={workingDir}
          />
        </Suspense>
      ) : (
        <div className="flex min-h-[400px] flex-1 items-center justify-center rounded-lg border border-dashed border-border bg-card/40 px-6 text-center text-sm text-muted-foreground">
          {t('description')}
        </div>
      )}
    </div>
  );
}
