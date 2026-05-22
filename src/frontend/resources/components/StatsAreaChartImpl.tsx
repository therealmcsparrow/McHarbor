// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { AreaChart, Area, XAxis, Tooltip, ResponsiveContainer } from 'recharts';
import type { StatsAreaChartProps } from './StatsAreaChart';

export function StatsAreaChartImpl({
  data,
  dataKey,
  secondaryDataKey,
  xAxisKey = 'timestamp',
  color = 'var(--primary)',
  secondaryColor = 'hsl(30 80% 55%)',
  formatValue,
  label,
  secondaryLabel,
  compact,
}: StatsAreaChartProps) {
  return (
    <div className={compact ? 'h-full' : 'my-2 flex-1'}>
      <ResponsiveContainer width="100%" height="100%" minWidth={0} minHeight={0}>
        <AreaChart data={data} margin={{ top: 0, right: 0, bottom: 0, left: 0 }}>
          <defs>
            <linearGradient id={`gradient-${dataKey}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor={color} stopOpacity={0.3} />
              <stop offset="95%" stopColor={color} stopOpacity={0} />
            </linearGradient>
            {secondaryDataKey && (
              <linearGradient id={`gradient-${secondaryDataKey}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={secondaryColor} stopOpacity={0.3} />
                <stop offset="95%" stopColor={secondaryColor} stopOpacity={0} />
              </linearGradient>
            )}
          </defs>
          {!compact && (
            <XAxis
              dataKey={xAxisKey}
              tick={false}
              axisLine={{ stroke: 'var(--border)' }}
              tickLine={false}
            />
          )}
          <Tooltip
            contentStyle={{
              backgroundColor: 'var(--card)',
              border: '1px solid var(--border)',
              borderRadius: '0.375rem',
              color: 'var(--foreground)',
              fontSize: '0.75rem',
            }}
            formatter={(value, name) => [
              formatValue ? formatValue(Number(value ?? 0)) : (value ?? 0),
              label && name === dataKey ? label : secondaryLabel && name === secondaryDataKey ? secondaryLabel : name,
            ]}
          />
          <Area
            type="monotone"
            dataKey={dataKey}
            stroke={color}
            fill={`url(#gradient-${dataKey})`}
            strokeWidth={compact ? 1.5 : 2}
          />
          {secondaryDataKey && (
            <Area
              type="monotone"
              dataKey={secondaryDataKey}
              stroke={secondaryColor}
              fill={`url(#gradient-${secondaryDataKey})`}
              strokeWidth={compact ? 1.5 : 2}
            />
          )}
        </AreaChart>
      </ResponsiveContainer>
    </div>
  );
}
