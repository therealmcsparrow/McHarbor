// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// HeaderBreadcrumb — the breadcrumb, in the topbar, where it belongs.
//
// A breadcrumb is orientation chrome, not content: same coordinates on every
// page, and it has to survive scroll. Rendered inside a detail page's hero
// card it scrolls away at exactly the moment a long page makes you forget
// where you are, which is the only moment it earns its keep.
//
// The topbar carries the breadcrumb *only*, never breadcrumb-plus-title: the
// header strip is h-14, and on a detail page the leaf and the <h1> would be
// the same string twice.

import { createPortal } from 'react-dom';
import { useHeaderSlot } from '@resources/hooks/useHeaderSlot';
import { Breadcrumb, type BreadcrumbCrumb, type BreadcrumbSibling } from './Breadcrumb';

type HeaderBreadcrumbProps = {
  /** Ancestors, root first. The last entry is the leaf. */
  crumbs: BreadcrumbCrumb[];
  /** Peers of the leaf — turns the leaf into a switcher. */
  siblings?: BreadcrumbSibling[];
  /** id of the current leaf within `siblings`. */
  currentId?: string;
};

export function HeaderBreadcrumb({ crumbs, siblings, currentId }: HeaderBreadcrumbProps) {
  const slot = useHeaderSlot();
  if (!slot) return null;

  return createPortal(
    <Breadcrumb crumbs={crumbs} siblings={siblings} currentId={currentId} className="min-w-0 flex-1" />,
    slot,
  );
}
