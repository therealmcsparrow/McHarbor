// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo, useState } from 'react';
import type { LogDisplayRecord } from './logRecords';

const RANGE_MILLISECONDS: Record<string, number> = {
  '5m': 300_000,
  '15m': 900_000,
  '1h': 3_600_000,
};

// The cutoff must track the clock each time the records change, so it is read inside the memo rather than frozen in state.
function currentTimeMs(): number {
  return Date.now();
}

/** Filter, level, time-range and ordering state shared by every log view, plus the records they leave visible. */
export function useLogFilter(records: LogDisplayRecord[]) {
  const [filter, setFilter] = useState('');
  const [regex, setRegex] = useState(false);
  const [level, setLevel] = useState('all');
  const [timeRange, setTimeRange] = useState('all');
  const [newestFirst, setNewestFirst] = useState(true);

  const matcher = useMemo(() => {
    if (!filter || !regex) return null;
    try {
      return new RegExp(filter, 'i');
    } catch {
      return false;
    }
  }, [filter, regex]);

  // Kernel and some journal lines carry no wall-clock time; a range would hide every one of them.
  const hasTimestamps = useMemo(() => records.some((record) => record.timestamp !== ''), [records]);

  const visible = useMemo(() => {
    const span = RANGE_MILLISECONDS[timeRange];
    const cutoff = span && hasTimestamps ? currentTimeMs() - span : 0;
    const matching = records.filter((record) => {
      if (level !== 'all' && record.level !== level) return false;
      if (cutoff && (!record.timestamp || Date.parse(record.timestamp) < cutoff)) return false;
      if (!filter) return true;
      if (matcher === false) return false;
      return matcher
        ? matcher.test(record.text.slice(0, 4096))
        : record.text.toLowerCase().includes(filter.toLowerCase());
    });
    return newestFirst ? [...matching].reverse() : matching;
  }, [filter, hasTimestamps, level, matcher, newestFirst, records, timeRange]);

  return {
    records: visible,
    filtering: filter !== '',
    barProps: {
      filter,
      onFilterChange: setFilter,
      regexError: matcher === false,
      regex,
      onRegexChange: setRegex,
      level,
      onLevelChange: setLevel,
      timeRange,
      onTimeRangeChange: setTimeRange,
      timeRangeDisabled: !hasTimestamps,
      newestFirst,
      onNewestFirstChange: setNewestFirst,
      visibleCount: visible.length,
    },
    newestFirst,
  };
}
