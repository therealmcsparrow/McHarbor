// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { createClientId } from '../utils/id';

type StableKeyEntry = {
  fingerprint: string;
  key: string;
};

export function useStableListKeys<T>(items: readonly T[], getFingerprint?: (item: T) => string): string[] {
  const [previousEntries, setEntries] = useState<StableKeyEntry[]>([]);
  const nextEntries: StableKeyEntry[] = [];
  const usedIndexes = new Set<number>();

  items.forEach((item, index) => {
    const fingerprint = getFingerprint?.(item) ?? '';
    let matchIndex = -1;

    if (fingerprint) {
      matchIndex = previousEntries.findIndex(
        (entry, previousIndex) => !usedIndexes.has(previousIndex) && entry.fingerprint === fingerprint,
      );
    }

    if (matchIndex === -1 && index < previousEntries.length && !usedIndexes.has(index)) {
      matchIndex = index;
    }

    if (matchIndex !== -1) {
      const matchedEntry = previousEntries[matchIndex];
      if (!matchedEntry) {
        nextEntries.push({
          fingerprint,
          key: `row-${createClientId()}`,
        });
        return;
      }
      usedIndexes.add(matchIndex);
      nextEntries.push({
        fingerprint,
        key: matchedEntry.key,
      });
      return;
    }

    nextEntries.push({
      fingerprint,
      key: `row-${createClientId()}`,
    });
  });

  const changed =
    nextEntries.length !== previousEntries.length ||
    nextEntries.some(
      (entry, index) =>
        entry.key !== previousEntries[index]?.key || entry.fingerprint !== previousEntries[index]?.fingerprint,
    );
  if (changed) setEntries(nextEntries);
  return nextEntries.map((entry) => entry.key);
}
