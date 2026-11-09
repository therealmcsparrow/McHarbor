// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const BACKUP_TEMP_DROP_IN_FILE = '/etc/systemd/system/mcharbor.service.d/backup-temp.conf';

export type BackupTempAbout = { native?: boolean; sandboxed?: boolean };

export type BackupTempRemedy =
  | { kind: 'sandboxed'; dropInFile: string; dropIn: string; chown: string }
  | { kind: 'container' }
  | { kind: 'none' }
  | { kind: 'invalid' };

// A control byte in a path would end the drop-in line or the shell command, so such a path gets no remedy.
const CONTROL = /[\u0000-\u001f\u007f]/;

const shellQuote = (path: string) => `'${path.replaceAll("'", "'\\''")}'`;

// systemd reads backslash, quote and percent specially inside a quoted value.
const systemdQuote = (path: string) =>
  `'${path.replaceAll('\\', '\\\\').replaceAll("'", "\\'").replaceAll('%', '%%')}'`;

/**
 * backupTempRemedy builds the commands that let a sandboxed native install write to `path`. The
 * text is made only from what the admin typed, and each parser's own quoting is applied.
 */
export function backupTempRemedy(path: string, about: BackupTempAbout | undefined): BackupTempRemedy {
  const typed = path.trim();
  if (typed === '' || about?.native === undefined) return { kind: 'none' };
  if (!about.native) return { kind: 'container' };
  if (!about.sandboxed) return { kind: 'none' };
  if (CONTROL.test(typed)) return { kind: 'invalid' };
  return {
    kind: 'sandboxed',
    dropInFile: BACKUP_TEMP_DROP_IN_FILE,
    dropIn: `[Service]\nReadWritePaths=${systemdQuote(typed)}`,
    chown: `sudo chown -R mcharbor:mcharbor ${shellQuote(typed)}`,
  };
}
