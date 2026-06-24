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

const TOP_PAD = 12;
const BOTTOM_PAD = 36;
const PADDING_X = 16;
const MIN_BAR_PX = 3;
// Fixed viewBox width: the SVG always fills the container via width="100%".
// Bars are distributed across this space so they grow when data is sparse.
const SVG_WIDTH = 700;
const MAX_BAR_WIDTH = 72;
const MIN_GAP = 4;

export function BarChart({ data, accent, height = 220, className }: BarChartProps) {
  const gradientId = useId();
  const plotHeight = height - TOP_PAD - BOTTOM_PAD;
  const zeroY = TOP_PAD + plotHeight;
  const labelY = zeroY + 20;
  const maxValue = Math.max(...data.map((d) => d.value), 0);
  const isEmpty = maxValue === 0;
  const colors = chartColors[accent];

  // Distribute bars evenly across the fixed viewBox plot area.
  const n = Math.max(data.length, 1);
  const slotWidth = (SVG_WIDTH - 2 * PADDING_X) / n;
  const barWidth = Math.min(MAX_BAR_WIDTH, Math.max(MIN_BAR_PX * 2, slotWidth - MIN_GAP));

  // Fixed stride so every inter-label gap is exactly the same number of slots.
  // ceil(n/8) guarantees at most 8 labels and perfectly uniform spacing — no
  // special-casing the last index, which was causing unequal gaps.
  const labelStride = Math.max(1, Math.ceil(data.length / 8));
  const showLabelAt = (index: number) => index % labelStride === 0;

  return (
    <div className={['h-full w-full overflow-hidden', className].filter(Boolean).join(' ')}>
      <svg
        width="100%"
        height="100%"
        viewBox={`0 0 ${SVG_WIDTH} ${height}`}
        preserveAspectRatio="none"
        style={{ display: 'block' }}
        role="img"
        aria-label="Bar chart"
      >
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={colors.start} stopOpacity="0.95" />
            <stop offset="70%" stopColor={colors.end} stopOpacity="0.62" />
            <stop offset="100%" stopColor={colors.end} stopOpacity="0.22" />
          </linearGradient>
        </defs>

        {[0.25, 0.5, 0.75, 1].map((mark) => (
          <line
            key={mark}
            x1={PADDING_X}
            x2={SVG_WIDTH - PADDING_X}
            y1={TOP_PAD + plotHeight * (1 - mark)}
            y2={TOP_PAD + plotHeight * (1 - mark)}
            stroke="var(--color-border)"
            strokeDasharray="4 6"
          />
        ))}

        {data.map((datum, index) => {
          const barPx =
            datum.value > 0
              ? Math.max((datum.value / maxValue) * plotHeight, MIN_BAR_PX)
              : 0;
          // Center each bar within its slot.
          const slotX = PADDING_X + index * slotWidth;
          const barX = slotX + (slotWidth - barWidth) / 2;
          const labelX = slotX + slotWidth / 2;

          return (
            <g key={`${index}-${datum.label}`}>
              {barPx > 0 && (
                <rect
                  x={barX}
                  y={zeroY - barPx}
                  width={barWidth}
                  height={barPx}
                  rx="5"
                  fill={`url(#${gradientId})`}
                />
              )}
              {showLabelAt(index) && (
                <text
                  x={labelX}
                  y={labelY}
                  textAnchor="middle"
                  fontSize="11"
                  fontWeight="500"
                  className="fill-text-mute-strong"
                >
                  {datum.label}
                </text>
              )}
            </g>
          );
        })}

        {isEmpty && (
          <text
            x={SVG_WIDTH / 2}
            y={TOP_PAD + plotHeight / 2}
            textAnchor="middle"
            dominantBaseline="middle"
            fontSize="12"
            fontWeight="500"
            className="fill-text-mute-strong"
          >
            No envelope activity in this period
          </text>
        )}
      </svg>
    </div>
  );
}
