// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import fs from 'node:fs';
import path from 'node:path';
import { chromium, request, type FullConfig } from '@playwright/test';

export const AUTH_STATE = path.join(import.meta.dirname, '..', '.results', 'playwright-auth.json');

type AuthStatus = { authDisabled?: boolean; needsSetup?: boolean };

async function authStatus(baseURL: string): Promise<AuthStatus> {
  const api = await request.newContext({ baseURL });
  try {
    for (let attempt = 0; attempt < 8; attempt += 1) {
      const response = await api.get('/api/auth/status');
      if (response.status() === 429) {
        await new Promise((resolve) => setTimeout(resolve, 10_000));
        continue;
      }
      if (!response.ok()) {
        throw new Error(
          `GET /api/auth/status answered ${response.status()}; is McHarbor reachable through MCHARBOR_API_TARGET?`,
        );
      }
      const body = (await response.json()) as { data?: AuthStatus };
      return body.data ?? {};
    }
    throw new Error('GET /api/auth/status stayed rate-limited (429) for over a minute');
  } finally {
    await api.dispose();
  }
}

async function savedSessionIsValid(baseURL: string): Promise<boolean> {
  if (!fs.existsSync(AUTH_STATE)) return false;
  const api = await request.newContext({ baseURL, storageState: AUTH_STATE });
  try {
    const response = await api.get('/api/auth/session');
    if (!response.ok()) return false;
    const body = (await response.json().catch(() => ({}))) as { success?: boolean; data?: unknown };
    return body.success === true && body.data !== null && body.data !== undefined;
  } finally {
    await api.dispose();
  }
}

export default async function globalSetup(config: FullConfig): Promise<void> {
  const baseURL = config.projects[0]?.use.baseURL;
  if (!baseURL) throw new Error('playwright config has no baseURL');
  fs.mkdirSync(path.dirname(AUTH_STATE), { recursive: true });

  if (process.env.MC_VISUAL_BYPASS_AUTH !== '1' && (await savedSessionIsValid(baseURL))) {
    return;
  }

  if (process.env.MC_VISUAL_BYPASS_AUTH === '1') {
    fs.writeFileSync(AUTH_STATE, JSON.stringify({ cookies: [], origins: [] }));
    return;
  }

  const currentStatus = await authStatus(baseURL);
  if (currentStatus.needsSetup)
    throw new Error('McHarbor has no account yet; finish first-run setup before running these suites');
  if (currentStatus.authDisabled) {
    fs.writeFileSync(AUTH_STATE, JSON.stringify({ cookies: [], origins: [] }));
    return;
  }

  const password = process.env.MC_ADMIN_PASSWORD ?? '';
  if (!password) throw new Error('authentication is enabled: set MC_ADMIN_USER and MC_ADMIN_PASSWORD');

  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ baseURL });
    for (let attempt = 1; ; attempt += 1) {
      await page.goto('/login');
      await page.getByLabel(/username/i).fill(process.env.MC_ADMIN_USER ?? 'admin');
      await page.getByLabel('Password', { exact: true }).fill(password);
      await page.getByRole('button', { name: /sign in|log in|submit/i }).click();
      const signedIn = await page
        .waitForURL(/\/dashboard/, { timeout: 15_000 })
        .then(() => true)
        .catch(() => false);
      if (signedIn) break;
      if (attempt >= 4)
        throw new Error('sign-in did not reach /dashboard after 4 attempts; check MC_ADMIN_USER / MC_ADMIN_PASSWORD');
      await page.waitForTimeout(20_000);
    }
    await page.context().storageState({ path: AUTH_STATE });
  } finally {
    await browser.close();
  }
}
