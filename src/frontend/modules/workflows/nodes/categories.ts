// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export const CATEGORY_COLORS: Record<string, { header: string; border: string }> = {
  trigger: { header: 'bg-emerald-600', border: 'border-emerald-500/30' },
  action: { header: 'bg-blue-600', border: 'border-blue-500/30' },
  logic: { header: 'bg-amber-600', border: 'border-amber-500/30' },
  utility: { header: 'bg-slate-600', border: 'border-slate-500/30' },
  integration: { header: 'bg-purple-600', border: 'border-purple-500/30' },
};

export const CATEGORY_TAG_COLORS: Record<string, string> = {
  trigger: 'bg-emerald-500/20 text-emerald-400',
  action: 'bg-blue-500/20 text-blue-400',
  logic: 'bg-amber-500/20 text-amber-400',
  utility: 'bg-slate-500/20 text-slate-400',
  integration: 'bg-purple-500/20 text-purple-400',
};

export const CATEGORY_GLOW_RGB: Record<string, string> = {
  trigger: '52, 211, 153',
  action: '96, 165, 250',
  logic: '251, 191, 36',
  utility: '100, 116, 139',
  integration: '167, 139, 250',
};
