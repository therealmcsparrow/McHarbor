// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef } from 'react';
import { useSearchParams } from 'react-router';

type UrlSearchStateOptions<T extends string> = {
  param?: string;
  allowed?: readonly T[];
  replace?: boolean;
};

let pending: URLSearchParams | null = null;

function useUrlParamWriter(replace: boolean) {
  const [searchParams, setSearchParams] = useSearchParams();
  const latest = useRef(searchParams);
  useEffect(() => {
    latest.current = searchParams;
  }, [searchParams]);

  const write = useCallback(
    (mutate: (params: URLSearchParams) => void) => {
      if (!pending) {
        pending = new URLSearchParams(latest.current);
        queueMicrotask(() => {
          pending = null;
        });
      }
      mutate(pending);
      setSearchParams(new URLSearchParams(pending), {
        replace,
        preventScrollReset: true,
      });
    },
    [replace, setSearchParams],
  );

  return [searchParams, write] as const;
}

export function useUrlSearchState<T extends string>(
  defaultValue: T,
  { param = 'q', allowed, replace = true }: UrlSearchStateOptions<T> = {},
) {
  const [searchParams, write] = useUrlParamWriter(replace);
  const rawValue = searchParams.get(param);
  const value = rawValue && (!allowed || allowed.includes(rawValue as T)) ? (rawValue as T) : defaultValue;

  const setValue = useCallback(
    (nextValue: T) => {
      write((current) => {
        if (!nextValue || nextValue === defaultValue) current.delete(param);
        else current.set(param, nextValue);
      });
    },
    [defaultValue, param, write],
  );

  return [value, setValue] as const;
}

export function useUrlNumberState(
  defaultValue: number,
  { param = 'page', replace = true }: Omit<UrlSearchStateOptions<string>, 'allowed'> = {},
) {
  const [searchParams, write] = useUrlParamWriter(replace);
  const rawValue = searchParams.get(param);
  const parsedValue = rawValue ? Number.parseInt(rawValue, 10) : defaultValue;
  const value = Number.isFinite(parsedValue) && parsedValue > 0 ? parsedValue : defaultValue;

  const setValue = useCallback(
    (nextValue: number) => {
      write((current) => {
        if (nextValue === defaultValue) current.delete(param);
        else current.set(param, String(nextValue));
      });
    },
    [defaultValue, param, write],
  );

  return [value, setValue] as const;
}
