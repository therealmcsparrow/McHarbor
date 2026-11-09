// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import { backupTempRemedy } from './backupTempRemedy';

const SANDBOXED = { native: true, sandboxed: true };
const UNSANDBOXED = { native: true, sandboxed: false };
const CONTAINER = { native: false, sandboxed: false };

function sandboxed(path: string) {
  const remedy = backupTempRemedy(path, SANDBOXED);
  if (remedy.kind !== 'sandboxed') {
    throw new Error(`expected a sandboxed remedy for ${JSON.stringify(path)}, got ${remedy.kind}`);
  }
  return remedy;
}

describe('backupTempRemedy', () => {
  it('gives the drop-in file, drop-in lines and chown command for a sandboxed native install', () => {
    const remedy = sandboxed('/srv/spool');
    expect(remedy.dropInFile).toBe('/etc/systemd/system/mcharbor.service.d/backup-temp.conf');
    expect(remedy.dropIn).toBe("[Service]\nReadWritePaths='/srv/spool'");
    expect(remedy.chown).toBe("sudo chown -R mcharbor:mcharbor '/srv/spool'");
  });

  it('gives no commands for an unsandboxed native install', () => {
    expect(backupTempRemedy('/srv/spool', UNSANDBOXED)).toEqual({ kind: 'none' });
  });

  it('gives only the mount hint for a container install', () => {
    expect(backupTempRemedy('/srv/spool', CONTAINER)).toEqual({ kind: 'container' });
  });

  it('fails closed when the install kind is unknown', () => {
    expect(backupTempRemedy('/srv/spool', undefined)).toEqual({ kind: 'none' });
    expect(backupTempRemedy('/srv/spool', {})).toEqual({ kind: 'none' });
  });

  it('gives nothing for an empty or blank path, which means reset to default', () => {
    expect(backupTempRemedy('', SANDBOXED)).toEqual({ kind: 'none' });
    expect(backupTempRemedy('   ', SANDBOXED)).toEqual({ kind: 'none' });
  });

  it('escapes a single quote so the shell sees one literal argument', () => {
    const remedy = sandboxed("/srv/it's");
    expect(remedy.chown).toBe("sudo chown -R mcharbor:mcharbor '/srv/it'\\''s'");
  });

  it('escapes a single quote in the drop-in so it cannot close the systemd quoting', () => {
    const remedy = sandboxed("/srv/a'b");
    const lines = remedy.dropIn.split('\n');
    expect(lines).toEqual(['[Service]', "ReadWritePaths='/srv/a\\'b'"]);
  });

  it('keeps a path that opens a command substitution inert inside single quotes', () => {
    const remedy = sandboxed('/srv/$(touch /tmp/pwn)');
    expect(remedy.chown).toBe("sudo chown -R mcharbor:mcharbor '/srv/$(touch /tmp/pwn)'");
    expect(remedy.dropIn).toBe("[Service]\nReadWritePaths='/srv/$(touch /tmp/pwn)'");
  });

  it('keeps backticks and spaces inert inside single quotes', () => {
    const remedy = sandboxed('/srv/my `id` spool');
    expect(remedy.chown).toBe("sudo chown -R mcharbor:mcharbor '/srv/my `id` spool'");
    expect(remedy.dropIn).toBe("[Service]\nReadWritePaths='/srv/my `id` spool'");
  });

  it('doubles a percent sign so systemd does not expand a specifier', () => {
    const remedy = sandboxed('/srv/%h/spool');
    expect(remedy.dropIn).toBe("[Service]\nReadWritePaths='/srv/%%h/spool'");
  });

  it('doubles a backslash in the drop-in', () => {
    const remedy = sandboxed('/srv/a\\b');
    expect(remedy.dropIn).toBe("[Service]\nReadWritePaths='/srv/a\\\\b'");
  });

  it.each([
    ['newline', '/srv/a\nExecStartPre=/bin/sh'],
    ['carriage return', '/srv/a\rExecStartPre=/bin/sh'],
    ['NUL', '/srv/a\0b'],
    ['tab', '/srv/a\tb'],
    ['escape', '/srv/a\u001bb'],
  ])('refuses a path holding a %s rather than emitting a command', (_name, path) => {
    expect(backupTempRemedy(path, SANDBOXED)).toEqual({ kind: 'invalid' });
  });

  it('never lets a hostile path add a line to the drop-in or the chown command', () => {
    const hostile = ["/a'; reboot; '", '/a$(id)', '/a`id`', '/a b', "/a'\\''b", '/a%n', '/a"b'];
    for (const path of hostile) {
      const remedy = sandboxed(path);
      expect(remedy.dropIn.split('\n')).toHaveLength(2);
      expect(remedy.chown.split('\n')).toHaveLength(1);
      expect(remedy.chown.startsWith("sudo chown -R mcharbor:mcharbor '")).toBe(true);
      expect(remedy.chown.endsWith("'")).toBe(true);
    }
  });
});
