// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { PlainButton } from '@resources/components/ui/PlainButton';
import { useTranslation } from 'react-i18next';
import { IconBrandDocker, IconCheck, IconChevronDown, IconServer } from '@tabler/icons-react';
import * as Popover from '@radix-ui/react-popover';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useSidebarStore } from '@resources/stores/sidebar';
import { STATUS_COLORS } from '@resources/theme/status-palette';
import { cn } from '@resources/utils/cn';

// `agent` is a *connection* type, not an orchestrator type — this test used to
// read `orchestratorType` and so never matched, which is why remote-agent
// environments always showed the Docker whale. Reading the field it meant all
// along both fixes the icon and survives orchestratorType being removed.
function EnvIcon({ connectionType, className }: { connectionType?: string; className?: string }) {
  if (connectionType === 'agent') return <IconServer className={className} />;
  return <IconBrandDocker className={className} />;
}

type EnvRecord = ReturnType<typeof useEnvironmentStore.getState>['environments'][number];

// Mirrors `deriveEnvironmentStatus` in the environments module rather than
// importing it: the shell must not depend on a feature module, and the store
// carries the same two fields the real one reads.
function isConnected(env?: EnvRecord): boolean {
  if (!env) return false;
  if (env.connectionType === 'agent') return env.agentStatus === 'connected';
  return Boolean(env.dockerVersion);
}

function StatusBar({ connected }: { connected: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'w-1 shrink-0 self-stretch rounded-r-sm',
        connected ? STATUS_COLORS.success.dot : STATUS_COLORS.danger.dot,
      )}
    />
  );
}

export function SidebarEnvironmentBadge() {
  const { t } = useTranslation('common');
  const environments = useEnvironmentStore((s) => s.environments);
  const currentId = useEnvironmentStore((s) => s.currentId);
  const setCurrentId = useEnvironmentStore((s) => s.setCurrentId);
  const collapsed = useSidebarStore((s) => s.collapsed);
  const currentEnv = environments.find((e) => e.id === currentId);
  const hasEnvs = environments.length > 0;

  const connected = isConnected(currentEnv);
  const ariaLabel = currentEnv
    ? `${t('sidebar.envSwitcherAria', {
        name: currentEnv.name,
      })} — ${t(connected ? 'status.connected' : 'status.disconnected')}`
    : t('sidebar.noEnvironments');

  if (collapsed) {
    return (
      <Popover.Root>
        <Popover.Trigger asChild>
          <PlainButton
            type="button"
            aria-label={ariaLabel}
            className={cn(
              'flex h-9 items-stretch text-muted-foreground transition-colors',
              'hover:bg-accent hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
            )}
          >
            <StatusBar connected={connected} />
            <span className="flex size-9 items-center justify-center">
              <EnvIcon connectionType={currentEnv?.connectionType} className="size-5" />
            </span>
          </PlainButton>
        </Popover.Trigger>
        <EnvironmentPopoverContent
          environments={environments}
          currentId={currentId}
          hasEnvs={hasEnvs}
          setCurrentId={setCurrentId}
        />
      </Popover.Root>
    );
  }

  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <PlainButton
          type="button"
          aria-label={ariaLabel}
          className={cn(
            'flex w-full items-stretch text-sm transition-colors',
            'hover:bg-accent focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
          )}
        >
          <StatusBar connected={connected} />
          <span className="flex min-w-0 flex-1 items-center gap-2.5 py-2.5 pr-2 pl-3">
            <EnvIcon connectionType={currentEnv?.connectionType} className="size-4 shrink-0 text-muted-foreground" />
            <span className="min-w-0 text-left">
              <span className="block text-[11px] leading-tight tracking-[0.15em] text-muted-foreground uppercase">
                {t('sidebar.envLabel')}
              </span>
              <span className="block truncate leading-tight font-semibold text-foreground">
                {currentEnv?.name ?? t('sidebar.noEnvironments')}
              </span>
            </span>
            <IconChevronDown className="ml-auto size-3.5 shrink-0 text-muted-foreground" />
          </span>
        </PlainButton>
      </Popover.Trigger>
      <EnvironmentPopoverContent
        environments={environments}
        currentId={currentId}
        hasEnvs={hasEnvs}
        setCurrentId={setCurrentId}
      />
    </Popover.Root>
  );
}

type EnvironmentPopoverContentProps = {
  environments: ReturnType<typeof useEnvironmentStore.getState>['environments'];
  currentId: string;
  hasEnvs: boolean;
  setCurrentId: (id: string) => void;
};

function EnvironmentPopoverContent({ environments, currentId, hasEnvs, setCurrentId }: EnvironmentPopoverContentProps) {
  const { t } = useTranslation('common');
  return (
    <Popover.Portal>
      <Popover.Content
        sideOffset={6}
        align="start"
        className="mc-glass-overlay mc-menu z-50 min-w-64 overflow-hidden p-1"
      >
        <p className="px-3 py-1.5 text-xs font-medium tracking-wider text-muted-foreground uppercase">
          {t('sidebar.environments')}
        </p>
        {hasEnvs ? (
          environments.map((env) => {
            const active = env.id === currentId;
            return (
              <Popover.Close asChild key={env.id}>
                <PlainButton
                  type="button"
                  onClick={() => setCurrentId(env.id)}
                  className={cn(
                    'flex w-full items-stretch text-sm text-foreground transition-colors',
                    'hover:bg-muted focus:outline-none focus-visible:bg-muted',
                    active && 'bg-muted/60',
                  )}
                  aria-current={active ? 'true' : undefined}
                >
                  <StatusBar connected={isConnected(env)} />
                  <span className="flex min-w-0 flex-1 items-center gap-2 py-2 pr-3 pl-2.5">
                    <EnvIcon connectionType={env.connectionType} className="size-4 shrink-0 text-muted-foreground" />
                    <span className="truncate">{env.name}</span>
                    <span className="ml-auto flex shrink-0 items-center gap-1">
                      {env.isDefault && (
                        <span className="bg-primary/10 px-2 py-0.5 text-[10px] font-medium tracking-wider text-primary uppercase">
                          {t('sidebar.envDefault')}
                        </span>
                      )}
                      {active && <IconCheck className="size-4 text-primary" />}
                    </span>
                  </span>
                </PlainButton>
              </Popover.Close>
            );
          })
        ) : (
          <p className="px-3 py-3 text-sm text-muted-foreground">{t('sidebar.noEnvironments')}</p>
        )}
      </Popover.Content>
    </Popover.Portal>
  );
}
