// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useCatalog } from '../hooks/useReporting';
import type { ReportType } from '../types';

// Catalog labels.
//
// A report type is either one of this plugin's own — labelled from the
// `reporting` bundle under `type.<key>.name` — or one another plugin
// contributed, which is labelled from *that* plugin's bundle. Every
// plugin namespace is merged into the same i18next instance by the
// host's loader, so resolving a contributed label is a matter of naming
// the right namespace rather than loading anything.
//
// The fallback chain matters more than it looks. A contributed type
// carries the English cover strings its provider declared, and those are
// a far better last resort than the raw key: an operator who has not
// translated a plugin should read "Kubernetes Cluster Health", not
// "kubernetes-cluster-health".

type Translate = (key: string, options?: Record<string, unknown>) => string;

function openLabel(t: Translate, key: string, ns: string | undefined, fallback: string): string {
  // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: keys and namespaces come from contributing plugins' catalog entries, which may carry no translation
  return t(key, { ns, defaultValue: fallback });
}

/** The studio card's title for one catalog entry. */
export function typeName(t: Translate, type: ReportType): string {
  if (type.provider) {
    return openLabel(
      t,
      type.label_key || `type.${type.key}.name`,
      type.label_ns || type.provider,
      type.title_en || type.key,
    );
  }
  return openLabel(t, `type.${type.key}.name`, undefined, type.key);
}

/** The studio card's description for one catalog entry. */
export function typeDescription(t: Translate, type: ReportType): string {
  if (type.provider) {
    return openLabel(
      t,
      type.description_key || `type.${type.key}.description`,
      type.label_ns || type.provider,
      type.subtitle_en || '',
    );
  }
  return openLabel(t, `type.${type.key}.description`, undefined, '');
}

/**
 * The shelf heading for a category.
 *
 * A provider reusing a built-in shelf ("platform") is labelled by this
 * plugin, because the shelf is this plugin's. A provider that minted its
 * own labels it itself — which is the only way a shelf named after
 * someone else's domain can be translated by the people who understand
 * it.
 */
export function categoryLabel(t: Translate, category: string, sample?: ReportType): string {
  if (sample?.provider && sample.category_label_key) {
    return openLabel(t, sample.category_label_key, sample.label_ns || sample.provider, category);
  }
  return openLabel(t, `category.${category}.label`, undefined, category);
}

export function categoryDescription(t: Translate, category: string, sample?: ReportType): string {
  if (sample?.provider && sample.category_description_key) {
    return openLabel(t, sample.category_description_key, sample.label_ns || sample.provider, '');
  }
  return openLabel(t, `category.${category}.description`, undefined, '');
}

/**
 * A scope-kind label, for the picker's "narrow this to a…" select.
 *
 * `ns` is the contributing plugin's namespace when the chosen report
 * type came from one; the built-in kinds resolve from this plugin's own
 * bundle exactly as before.
 */
export function subjectKindLabel(t: Translate, kind: string, ns?: string): string {
  return openLabel(t, `subject.${kind}`, ns, kind);
}

export function subjectAllLabel(t: Translate, kind: string, ns?: string): string {
  return openLabel(t, `subjectAll.${kind}`, ns, kind);
}

/**
 * useTypeName resolves a stored report's type key to a display name.
 *
 * The library, the schedule list and the schedule editor hold a bare
 * type key rather than a catalog entry, and a contributed key rendered
 * raw reads as an internal identifier. Looking the key up in the catalog
 * costs one already-cached query and gives every one of those surfaces
 * the same name the studio card shows.
 */
export function useTypeName(): (key: string) => string {
  const { t } = useTranslation('reporting');
  const catalog = useCatalog();
  const byKey = useMemo(() => {
    const map = new Map<string, ReportType>();
    for (const type of catalog.data?.types ?? []) map.set(type.key, type);
    return map;
  }, [catalog.data]);

  return useCallback(
    (key: string) => {
      const type = byKey.get(key);
      // A key with no catalog entry is a report whose contributing
      // plugin has since been removed. The stored row is still real, so
      // it keeps its key rather than disappearing from the library.
      if (!type) return openLabel(t, `type.${key}.name`, undefined, key);
      return typeName(t, type);
    },
    [byKey, t],
  );
}
