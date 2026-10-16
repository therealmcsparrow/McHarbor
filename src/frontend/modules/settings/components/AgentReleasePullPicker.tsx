// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCloudDownload } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import type { AgentAvailableVersion, AgentRelease } from '../hooks/useAgentReleases';

type AgentReleasePullPickerProps = {
  available: AgentAvailableVersion[];
  releases: AgentRelease[];
  latest: string;
  latestBuild?: string;
  pending: boolean;
  onPull: (version: string, build?: string) => void;
};

// One store row per version, so a build is unavailable while its version is
// downloading, and only the exact stored build is redundant; a failed row
// leaves every build of that version on offer again.
function storedOrBusy(releases: AgentRelease[], version: string, build: string) {
  return releases.some(
    (r) => r.version === version && r.status !== 'failed' && (r.status === 'pulling' || r.build === build),
  );
}

const optionValue = (v: AgentAvailableVersion) => `${v.version}@${v.build ?? ''}`;

export function AgentReleasePullPicker({
  available,
  releases,
  latest,
  latestBuild = '',
  pending,
  onPull,
}: AgentReleasePullPickerProps) {
  const { t } = useTranslation('settings');
  const [choice, setChoice] = useState('');

  const options = useMemo(() => {
    return available
      .filter((v) => {
        const build = v.build ?? '';
        // The card's own download button already offers the newest build.
        if (v.version === latest && build === latestBuild) return false;
        return !storedOrBusy(releases, v.version, build);
      })
      .map((v) => ({
        value: optionValue(v),
        label: v.version,
        description: v.build || undefined,
        pull: v,
      }));
  }, [available, releases, latest, latestBuild]);

  if (options.length === 0) return null;
  const selected = options.find((o) => o.value === choice);

  return (
    <div className="flex items-center gap-2">
      <Select
        value={selected?.value ?? ''}
        onChange={setChoice}
        options={options}
        placeholder={t('agentReleases.pullVersionPlaceholder')}
        ariaLabel={t('agentReleases.pullVersionLabel')}
        className="w-56"
      />
      <Button
        size="sm"
        variant="outline"
        permission={PERM.UPDATES_MANAGE}
        disabled={!selected || pending}
        onClick={() => {
          if (!selected) return;
          onPull(selected.pull.version, selected.pull.build || undefined);
          setChoice('');
        }}
      >
        {pending ? (
          <Spinner className="mr-1.5 h-4 w-4" />
        ) : (
          <IconCloudDownload className="mr-1.5 h-4 w-4" aria-hidden="true" />
        )}
        {t('agentReleases.pullVersion')}
      </Button>
    </div>
  );
}
