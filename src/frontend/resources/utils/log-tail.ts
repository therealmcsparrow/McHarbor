// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

const ansiPattern = /\u001B\[[0-?]*[ -/]*[@-~]/g;

export function extractLogTail(raw: string | undefined, limit = 20): string[] {
  if (!raw) {
    return [];
  }

  return raw
    .replace(ansiPattern, '')
    .split(/\r?\n/)
    .map((line) => line.trimEnd())
    .filter(Boolean)
    .slice(-limit);
}
