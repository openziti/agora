import { useId } from 'react';

import type { SurfaceAccent } from './StatCard';

export type BarChartDatum = {
  label: string;
  value: number;
};

export type BarChartProps = {
  data: BarChartDatum[];
  accent: SurfaceAccent;
  height?: number;
  className?: string;
};

const chartColors: Record<SurfaceAccent, { start: string; end: string }> = {
  agora: { start: 'var(--color-brand-agora)', end: 'var(--color-brand-agora-end)' },
  llm: { start: 'var(--color-brand-llm)', end: 'var(--color-brand-llm-end)' },
  mcp: { start: 'var(--color-brand-mcp)', end: 'var(--color-brand-mcp-end)' },
  success: { start: 'var(--color-success)', end: 'var(--color-success-strong)' },
  warning: { start: 'var(--color-warning)', end: 'var(--color-warning-strong)' },
  danger: { start: 'var(--color-danger)', end: 'var(--color-danger)' },
  info: { start: 'var(--color-info)', end: 'var(--color-brand-agora-end)' },
  neutral: { start: 'var(--color-text-mute-2)', end: 'var(--color-border-strong)' },
};

export function BarChart({ data, accent, height = 220, className }: BarChartProps) {
  const gradientId = useId();
  const max = Math.max(...data.map((datum) => datum.value), 1);
  const width = Math.max(data.length * 48, 320);
  const chartHeight = height - 28;
  const gap = 10;
  const paddingX = 16;
  const barWidth = Math.max((width - paddingX * 2 - gap * (data.length - 1)) / data.length, 12);
  const colors = chartColors[accent];

  return (
    <div className={['w-full overflow-hidden', className].filter(Boolean).join(' ')}>
      <svg className="block w-full" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="Bar chart">
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={colors.start} stopOpacity="0.95" />
            <stop offset="70%" stopColor={colors.end} stopOpacity="0.62" />
            <stop offset="100%" stopColor={colors.end} stopOpacity="0.22" />
          </linearGradient>
        </defs>

        {[0.25, 0.5, 0.75, 1].map((mark) => {
          const y = 12 + chartHeight * (1 - mark);

          return (
            <line
              key={mark}
              x1={paddingX}
              x2={width - paddingX}
              y1={y}
              y2={y}
              stroke="var(--color-border)"
              strokeDasharray="4 6"
            />
          );
        })}

        {data.map((datum, index) => {
          const barHeight = Math.max((datum.value / max) * (chartHeight - 20), 4);
          const x = paddingX + index * (barWidth + gap);
          const y = chartHeight + 4 - barHeight;

          return (
            <g key={datum.label}>
              <rect x={x} y={y} width={barWidth} height={barHeight} rx="5" fill={`url(#${gradientId})`} />
              <text
                x={x + barWidth / 2}
                y={height - 7}
                textAnchor="middle"
                className="fill-text-mute text-[0.625rem]"
              >
                {datum.label}
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}
