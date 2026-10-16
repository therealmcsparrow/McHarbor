// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import i18n from 'i18next';
import { http, HttpResponse } from 'msw';
import { beforeAll, beforeEach, describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { RunbookStepParams } from './RunbookStepParams';
import type { RunbookTargetKind } from '@resources/types/runbookCatalog';

const spec = { name: 'cluster', type: 'target' as const, target: 'widget_cluster', required: false };
const base: RunbookTargetKind = {
  kind: 'widget_cluster',
  labelKey: 'runbook.targets.widget_cluster',
  labelNs: 'widgets',
};

beforeAll(() => {
  i18n.addResourceBundle(
    'en',
    'widgets',
    { runbook: { targets: { widget_cluster: 'Widget cluster' }, clusterDefault: 'Default widget cluster' } },
    true,
    true,
  );
});

beforeEach(() => {
  server.use(http.get('*/api/runbooks/targets', () => HttpResponse.json({ success: true, data: [] })));
});

function show(targets: RunbookTargetKind[]) {
  renderWithProviders(
    <RunbookStepParams specs={[spec]} targets={targets} values={{}} envId="env-1" onChange={() => {}} />,
  );
}

describe('RunbookStepParams empty selector label', () => {
  it('renders the empty label the plugin declares for its target', async () => {
    show([{ ...base, emptyLabelKey: 'runbook.clusterDefault' }]);
    expect(await screen.findByText('Default widget cluster')).toBeInTheDocument();
    expect(screen.queryByText("The run's own subject")).not.toBeInTheDocument();
  });

  it('falls back to the run subject when the target declares no empty label', async () => {
    show([base]);
    expect(await screen.findByText("The run's own subject")).toBeInTheDocument();
    expect(screen.queryByText('Default widget cluster')).not.toBeInTheDocument();
  });
});
