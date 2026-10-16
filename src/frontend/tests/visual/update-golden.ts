// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.
//
// `npm run test:visual:update` regenerates the committed golden
// baselines. Run after an intentional layout / colour / theme change.
//
// This is a thin wrapper over `playwright test --update-snapshots`
// so developers don't have to remember the long command. The actual
// baseline files live in `tests/visual/run-visual.spec.ts-snapshots/`
// (Playwright's default snapshot directory for that spec).

// We re-exec Playwright with the visual config + `--update-snapshots`.
// Doing it via a child process keeps the wrapper dependency-free and
// inherits any future flags the team adds to the test command.
import { spawn } from 'node:child_process';

const args = ['playwright', 'test', '--config=playwright.visual.config.ts', '--update-snapshots'];

console.info(`[visual:update] ${process.platform === 'win32' ? 'npx' : 'npx'} ${args.join(' ')}`);

const child = spawn('npx', args, {
  stdio: 'inherit',
  env: process.env,
  shell: process.platform === 'win32',
});

child.on('exit', (code) => {
  if (code === 0) {
    console.info(
      `[visual:update] Done. Review with \`git diff tests/visual/run-visual.spec.ts-snapshots/\` and commit alongside the visual change.`,
    );
  }
  process.exit(code ?? 1);
});
