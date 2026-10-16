// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import fs from 'node:fs';
import path from 'node:path';
import { test, expect, type APIRequestContext, type Page } from '@playwright/test';

type Violation = {
  directive: string;
  blockedURI: string;
  sourceFile: string;
  line: number;
  sample: string;
  disposition: string;
};

type PageResult = {
  name: string;
  path: string;
  finalURL: string;
  reached: boolean;
  violations: Violation[];
  consoleCSP: string[];
  pageErrors: string[];
  note: string;
};

type Step = {
  name: string;
  path: string;
  act?: (page: Page) => Promise<string>;
};

const BASE = process.env.MC_CSP_BASE_URL ?? '';
const API_KEY = process.env.MC_CSP_API_KEY ?? '';
const READ_ONLY_METHODS = new Set(['GET', 'HEAD', 'OPTIONS']);
const CSP_TEXT = /content.security.policy/i;
const RESULTS_DIR = path.join(import.meta.dirname, '..', '..', '.results', 'csp');

test.skip(!BASE || !API_KEY, 'opt-in: set MC_CSP_BASE_URL and MC_CSP_API_KEY to walk a live instance');

async function apiData<T>(api: APIRequestContext, url: string): Promise<T | undefined> {
  const response = await api.get(url).catch(() => undefined);
  if (!response || !response.ok()) return undefined;
  const body = (await response.json().catch(() => ({}))) as { data?: T };
  return body.data;
}

async function settle(page: Page): Promise<void> {
  await page.waitForLoadState('domcontentloaded');
  await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => undefined);
  await page.waitForTimeout(1_500);
}

async function probeWebSocket(page: Page): Promise<string> {
  const outcome = await page.evaluate(
    () =>
      new Promise<string>((resolve) => {
        const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
        try {
          const socket = new WebSocket(`${scheme}://${window.location.host}/api/csp-probe/ws`);
          const done = (state: string) => {
            socket.close();
            resolve(state);
          };
          socket.onopen = () => done('opened');
          socket.onerror = () => done('refused by server');
          setTimeout(() => done('timeout'), 5_000);
        } catch (error) {
          resolve(`constructor threw: ${String(error)}`);
        }
      }),
  );
  return `same-origin wss probe: ${outcome}`;
}

async function exportCompliance(page: Page, format: RegExp): Promise<string> {
  await page.getByRole('button', { name: /download report/i }).click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor();
  await dialog.getByRole('button', { name: /^format$/i }).click();
  await page.getByRole('button', { name: format }).click();
  const download = page.waitForEvent('download', { timeout: 30_000 }).then((d) => d.suggestedFilename());
  await dialog.getByRole('button', { name: /^download$/i }).click();
  const file = await download.catch((error: unknown) => `no download: ${String(error)}`);
  await page.keyboard.press('Escape').catch(() => undefined);
  return `export ${format.source}: ${file}`;
}

async function clickEveryTab(page: Page): Promise<string> {
  const tabs = page.getByRole('tab');
  const count = await tabs.count();
  for (let i = 0; i < count; i += 1) {
    await tabs
      .nth(i)
      .click()
      .catch(() => undefined);
    await page.waitForTimeout(800);
  }
  return `${count} tabs opened`;
}

async function openPluginForm(page: Page): Promise<string> {
  await page
    .getByRole('button', { name: /add communication/i })
    .first()
    .click();
  const dialog = page.getByRole('dialog');
  await dialog.waitFor();
  await dialog
    .getByRole('button', { name: /telegram/i })
    .first()
    .click();
  await page.waitForTimeout(2_500);
  const mounted = await dialog.locator('input, textarea, select').count();
  await page.keyboard.press('Escape').catch(() => undefined);
  return `comm-telegram config_form outlet mounted with ${mounted} fields`;
}

async function openVMConsole(page: Page): Promise<string> {
  const firstRow = page
    .locator('table tbody tr, [role="row"]')
    .filter({ hasNot: page.locator('th') })
    .first();
  if ((await firstRow.count()) === 0) return 'no VM listed';
  await firstRow.click();
  await settle(page);
  const consoleTab = page.getByRole('tab', { name: /console/i });
  if ((await consoleTab.count()) === 0) return `opened ${page.url()} but no console tab`;
  await consoleTab.first().click();
  await page.waitForTimeout(2_000);
  return `console tab loaded at ${page.url()}`;
}

async function discoverSteps(api: APIRequestContext): Promise<Step[]> {
  const steps: Step[] = [
    { name: 'dashboard', path: '/dashboard', act: probeWebSocket },
    { name: 'containers list', path: '/containers' },
  ];

  const environments = (await apiData<Array<{ id: string; name: string }>>(api, '/api/environments')) ?? [];
  let containerStep: Step | undefined;
  for (const env of environments) {
    const containers =
      (await apiData<Array<{ Id: string; State: string }>>(api, `/api/containers?env=${env.id}`)) ?? [];
    const running = containers.find((c) => c.State === 'running') ?? containers[0];
    if (running) {
      containerStep = { name: `container detail (${env.name})`, path: `/containers/${running.Id}?env=${env.id}` };
      steps.push(containerStep, {
        name: `container terminal tab (${env.name})`,
        path: `/containers/${running.Id}?env=${env.id}&tab=terminal`,
      });
      break;
    }
  }
  steps.push({ name: 'terminal page', path: '/terminal' });
  for (const env of environments) {
    steps.push({ name: `host terminal tab (${env.name})`, path: `/environments/${env.id}?tab=terminal` });
  }

  const workflows = await apiData<{ items?: Array<{ id: string }> }>(api, '/api/workflows');
  steps.push({ name: 'workflows list', path: '/workflows' });
  const firstWorkflow = workflows?.items?.[0];
  if (firstWorkflow) steps.push({ name: 'workflow editor', path: `/workflows/${firstWorkflow.id}` });

  steps.push(
    { name: 'settings (every tab)', path: '/settings', act: clickEveryTab },
    { name: 'plugin form outlet', path: '/settings?tab=communications', act: openPluginForm },
    { name: 'profile (every tab)', path: '/profile', act: clickEveryTab },
    { name: 'security', path: '/security' },
    { name: 'compliance + docx export', path: '/enterprise/compliance', act: (p) => exportCompliance(p, /word/i) },
    { name: 'compliance + pdf export', path: '/enterprise/compliance', act: (p) => exportCompliance(p, /pdf/i) },
    { name: 'app store', path: '/store' },
    { name: 'stacks', path: '/stacks' },
    { name: 'images', path: '/images' },
    { name: 'logs', path: '/logs' },
    { name: 'audit', path: '/audit' },
  );

  const plugins =
    (await apiData<Array<{ slug: string; enabled: boolean; loaded: boolean; bundle_mtime?: number }>>(
      api,
      '/api/plugins/loader/status',
    )) ?? [];
  for (const plugin of plugins) {
    if (!plugin.enabled || !plugin.loaded || !plugin.slug.startsWith('addon-')) continue;
    steps.push({ name: `plugin ${plugin.slug}`, path: `/plugins/${plugin.slug}` });
    if (plugin.slug === 'addon-kvm') {
      steps.push({ name: 'plugin addon-kvm VM console', path: '/plugins/addon-kvm', act: openVMConsole });
    }
  }
  return steps;
}

test('the default Content-Security-Policy admits the whole host UI', async ({ page, playwright }) => {
  const origin = new URL(BASE).origin;
  const api = await playwright.request.newContext({
    baseURL: origin,
    ignoreHTTPSErrors: true,
    extraHTTPHeaders: { Authorization: `Bearer ${API_KEY}` },
  });
  const blockedMutations: string[] = [];
  let current: PageResult | undefined;

  await page.context().route(`${origin}/**`, async (route) => {
    const request = route.request();
    if (!READ_ONLY_METHODS.has(request.method())) {
      blockedMutations.push(`${request.method()} ${new URL(request.url()).pathname}`);
      await route.abort('blockedbyclient');
      return;
    }
    await route.continue({ headers: { ...request.headers(), authorization: `Bearer ${API_KEY}` } });
  });
  await page.exposeBinding('__mcCspReport', (_source, violation: Violation) => {
    current?.violations.push(violation);
  });
  await page.addInitScript(() => {
    document.addEventListener('securitypolicyviolation', (event) => {
      const report = (window as unknown as { __mcCspReport?: (v: unknown) => void }).__mcCspReport;
      report?.({
        directive: event.effectiveDirective || event.violatedDirective,
        blockedURI: event.blockedURI,
        sourceFile: event.sourceFile,
        line: event.lineNumber,
        sample: event.sample,
        disposition: event.disposition,
      });
    });
  });
  page.on('console', (message) => {
    if (CSP_TEXT.test(message.text())) current?.consoleCSP.push(message.text());
  });
  page.on('pageerror', (error) => {
    current?.pageErrors.push(error.message);
  });

  const headers = (await api.get('/')).headers();
  const policy = headers['content-security-policy'] ?? headers['content-security-policy-report-only'] ?? '';
  expect(policy, 'the instance sends no Content-Security-Policy').not.toBe('');

  const canary: PageResult = {
    name: 'canary',
    path: '/dashboard',
    finalURL: '',
    reached: false,
    violations: [],
    consoleCSP: [],
    pageErrors: [],
    note: '',
  };
  current = canary;
  await page.goto(canary.path);
  await settle(page);
  await page.evaluate(() => fetch('https://csp-canary.invalid/probe').catch(() => undefined));
  await page.waitForTimeout(1_000);
  expect(
    canary.violations.map((v) => v.directive),
    'the collector missed a deliberate connect-src violation, so a clean walk would prove nothing',
  ).toContain('connect-src');

  const steps = await discoverSteps(api);
  const results: PageResult[] = [];
  fs.mkdirSync(RESULTS_DIR, { recursive: true });
  for (const [index, step] of steps.entries()) {
    current = {
      name: step.name,
      path: step.path,
      finalURL: '',
      reached: false,
      violations: [],
      consoleCSP: [],
      pageErrors: [],
      note: '',
    };
    results.push(current);
    try {
      await page.goto(step.path);
      await settle(page);
      if (step.act) current.note = await step.act(page);
      await settle(page);
      current.finalURL = page.url();
      current.reached = !/\/login(\?|$)/.test(new URL(page.url()).pathname);
      await page.screenshot({ path: path.join(RESULTS_DIR, `${String(index).padStart(2, '0')}.png`) });
    } catch (error) {
      current.finalURL = page.url();
      current.note = `navigation failed: ${String(error).split('\n')[0]}`;
    }
  }
  current = undefined;

  fs.writeFileSync(
    path.join(RESULTS_DIR, 'violations.json'),
    `${JSON.stringify({ policy, blockedMutations, results }, null, 2)}\n`,
  );
  const rows = results.map((r) => {
    const found = r.violations.map((v) => `${v.directive} → ${v.blockedURI || '(inline)'}`).join('<br>') || 'none';
    return `| ${r.name} | \`${r.path}\` | ${r.reached ? 'yes' : 'no'} | ${found} | ${r.consoleCSP.length} | ${r.note.replace(/\|/g, '/')} |`;
  });
  fs.writeFileSync(
    path.join(RESULTS_DIR, 'violations.md'),
    [
      `Policy: \`${policy}\``,
      '',
      '| Page | Path | Reached | Violations (directive → blocked URI) | CSP console lines | Note |',
      '|---|---|---|---|---|---|',
      ...rows,
      '',
      `Blocked mutating requests: ${blockedMutations.length ? blockedMutations.join(', ') : 'none'}`,
      '',
    ].join('\n'),
  );
  await api.dispose();

  for (const r of results) {
    expect.soft(r.violations, `${r.name} (${r.path}) raised CSP violations`).toEqual([]);
    expect.soft(r.consoleCSP, `${r.name} (${r.path}) logged CSP console errors`).toEqual([]);
  }
});
