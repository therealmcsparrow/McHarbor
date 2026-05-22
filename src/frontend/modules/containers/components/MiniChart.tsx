// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Sparkline } from '@resources/components/Sparkline';

type MiniChartProps = {
  label: string;
  value: string;
  data: number[];
  color: string;
};

export function MiniChart({ label, value, data, color }: MiniChartProps) {
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-baseline justify-between">
        <span className="text-[10px] text-muted-foreground">{label}</span>
        <span className="text-[10px] tabular-nums text-foreground">{value}</span>
      </div>
      <div className="h-5">
        <Sparkline data={data.length >= 2 ? data : [0, 0]} width={120} height={20} color={color} />
      </div>
    </div>
  );
}
