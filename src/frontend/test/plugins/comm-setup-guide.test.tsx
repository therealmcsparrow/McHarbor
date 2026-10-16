// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import i18next from 'i18next';
import { beforeAll, describe, expect, it } from 'vitest';
import { RichText } from '../../../plugins/Plugins/comm-teams/frontend/src/RichText';
import { SetupGuide as SignalGuide } from '../../../plugins/Plugins/comm-signal/frontend/src/SetupGuide';
import { SetupGuide as TeamsGuide } from '../../../plugins/Plugins/comm-teams/frontend/src/SetupGuide';
import signalEn from '../../../plugins/Plugins/comm-signal/i18n/en/common.json';
import teamsEn from '../../../plugins/Plugins/comm-teams/i18n/en/common.json';
import { i18n, initTestI18n } from '../i18n';

const bundles = import.meta.glob<Record<string, unknown>>(
  '../../../plugins/Plugins/{comm,storage}-*/i18n/*/common.json',
  {
    import: 'default',
    eager: true,
  },
);

const RAW_TAG = /<\/?(?:b|code|warn)>/;

function strings(node: unknown, out: string[] = []): string[] {
  if (typeof node === 'string') out.push(node);
  else if (node && typeof node === 'object') for (const v of Object.values(node)) strings(v, out);
  return out;
}

// Mirrors the host's makeT shim: the guide only ever sees a string-returning `t` in the plugin's own namespace.
const teamsT = (key: string) => String(i18next.t(key, { ns: 'comm-teams' }));

beforeAll(async () => {
  await initTestI18n();
  i18next.addResourceBundle('en', 'comm-teams', teamsEn, true, true);
  i18n.addResourceBundle('en', 'comm-signal', signalEn, true, true);
});

describe('plugin setup guides', () => {
  it('renders bold, code and warn spans from the string t the host passes', () => {
    const { container } = render(<TeamsGuide t={teamsT} />);
    expect(container.querySelector('strong')?.textContent).toBe('Connectors');
    expect(screen.getByText('5059C9').tagName).toBe('CODE');
    const warn = screen.getByText(/you will not see it again/);
    expect(warn.className).toContain('text-warning');
    expect(container.querySelector('ol')?.className).toContain('text-muted-foreground');
    expect(container.textContent).not.toMatch(RAW_TAG);
  });

  it('renders markup in a guide that reads its own i18n namespace', () => {
    const { container } = render(<SignalGuide />);
    expect(screen.getByText('bbernhard/signal-cli-rest-api').tagName).toBe('CODE');
    expect(container.querySelector('strong')).not.toBeNull();
    expect(container.textContent).not.toMatch(RAW_TAG);
  });

  it('drops a stray or unbalanced tag instead of printing it', () => {
    const { container } = render(<RichText text="Press <b>Save and </code> then <warn>wait" />);
    expect(container.textContent).toBe('Press Save and  then wait');
  });

  it('keeps angle-bracket placeholders inside code literal', () => {
    render(<RichText text="Use <code>http://<gateway>:8080</code> here" />);
    expect(screen.getByText('http://<gateway>:8080').tagName).toBe('CODE');
  });

  it('keeps every marked guide string balanced in all six locales', () => {
    const marked = Object.entries(bundles).flatMap(([path, bundle]) =>
      strings(bundle)
        .filter((s) => RAW_TAG.test(s))
        .map((s) => ({ path, s })),
    );
    expect(marked.length).toBeGreaterThan(500);
    const unbalanced = marked.filter(({ s }) => RAW_TAG.test(s.replace(/<(b|code|warn)>[\s\S]*?<\/\1>/g, '')));
    expect(unbalanced).toEqual([]);
  });
});
