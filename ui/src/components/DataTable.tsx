import { useMemo, useState, type ReactNode } from 'react';
import { ArrowDownRight, ArrowUpRight, MoreHorizontal } from 'lucide-react';

import { StatusPill, type StatusPillStatus } from './StatusPill';

export type DataTableCellKind = 'pill' | 'mono' | 'plain';

export type DataTablePillValue = {
  status: StatusPillStatus;
  label: string;
};

export type DataTableColumn<T> = {
  id: string;
  header: string;
  accessor: (row: T) => ReactNode | DataTablePillValue;
  kind?: DataTableCellKind;
  sortable?: boolean;
  sortValue?: (row: T) => string | number;
  align?: 'left' | 'right';
};

export type DataTableProps<T> = {
  columns: DataTableColumn<T>[];
  rows: T[];
  getRowKey?: (row: T, index: number) => string;
  onRowClick?: (row: T) => void;
  actions?: (row: T) => ReactNode;
  emptyState?: ReactNode;
  className?: string;
};

type SortState = {
  columnId: string;
  direction: 'asc' | 'desc';
};

function isPillValue(value: ReactNode | DataTablePillValue): value is DataTablePillValue {
  return typeof value === 'object' && value !== null && 'status' in value && 'label' in value;
}

function compareValues(left: string | number, right: string | number) {
  if (typeof left === 'number' && typeof right === 'number') {
    return left - right;
  }

  return String(left).localeCompare(String(right));
}

function defaultSortValue<T>(column: DataTableColumn<T>, row: T) {
  const value = column.accessor(row);

  if (typeof value === 'string' || typeof value === 'number') {
    return value;
  }

  if (isPillValue(value)) {
    return value.label;
  }

  return '';
}

export function DataTable<T>({
  columns,
  rows,
  getRowKey,
  onRowClick,
  actions,
  emptyState,
  className,
}: DataTableProps<T>) {
  const firstSortableColumn = columns.find((column) => column.sortable);
  const [sort, setSort] = useState<SortState | undefined>(
    firstSortableColumn ? { columnId: firstSortableColumn.id, direction: 'asc' } : undefined,
  );

  const sortedRows = useMemo(() => {
    if (!sort) {
      return rows;
    }

    const column = columns.find((candidate) => candidate.id === sort.columnId);

    if (!column) {
      return rows;
    }

    return [...rows].sort((left, right) => {
      const leftValue = column.sortValue?.(left) ?? defaultSortValue(column, left);
      const rightValue = column.sortValue?.(right) ?? defaultSortValue(column, right);
      const comparison = compareValues(leftValue, rightValue);

      return sort.direction === 'asc' ? comparison : -comparison;
    });
  }, [columns, rows, sort]);

  function toggleSort(column: DataTableColumn<T>) {
    if (!column.sortable) {
      return;
    }

    setSort((current) => ({
      columnId: column.id,
      direction: current?.columnId === column.id && current.direction === 'asc' ? 'desc' : 'asc',
    }));
  }

  function renderCell(column: DataTableColumn<T>, row: T) {
    const value = column.accessor(row);

    if (column.kind === 'pill' && isPillValue(value)) {
      return <StatusPill status={value.status} label={value.label} />;
    }

    if (column.kind === 'mono') {
      return <span className="font-mono text-table text-text">{isPillValue(value) ? value.label : value}</span>;
    }

    return isPillValue(value) ? value.label : value;
  }

  if (rows.length === 0) {
    return <div className={className}>{emptyState}</div>;
  }

  return (
    <div className={['overflow-hidden rounded-card border border-border bg-panel', className].filter(Boolean).join(' ')}>
      <div className="overflow-x-auto">
        <table className="min-w-full border-collapse text-left">
          <thead className="bg-panel-subtle">
            <tr>
              {columns.map((column) => {
                const isSorted = sort?.columnId === column.id;
                const SortIcon = isSorted && sort.direction === 'desc' ? ArrowDownRight : ArrowUpRight;

                return (
                  <th
                    key={column.id}
                    scope="col"
                    className={[
                      'border-b border-border px-4 py-3 text-label font-medium uppercase text-text-mute',
                      column.align === 'right' ? 'text-right' : 'text-left',
                    ].join(' ')}
                  >
                    {column.sortable ? (
                      <button
                        type="button"
                        className="inline-flex items-center gap-1 rounded-pill text-label font-medium uppercase text-text-mute hover:text-text-mute-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
                        onClick={() => toggleSort(column)}
                      >
                        {column.header}
                        <SortIcon size={13} aria-hidden="true" className={isSorted ? 'text-brand-agora' : 'text-text-mute-2'} />
                      </button>
                    ) : (
                      column.header
                    )}
                  </th>
                );
              })}
              {actions ? <th scope="col" className="border-b border-border px-4 py-3" /> : null}
            </tr>
          </thead>
          <tbody className="divide-y divide-border bg-panel">
            {sortedRows.map((row, index) => {
              const rowKey = getRowKey?.(row, index) ?? String(index);
              const clickable = Boolean(onRowClick);

              return (
                <tr
                  key={rowKey}
                  className={clickable ? 'cursor-pointer hover:bg-panel-subtle' : undefined}
                  onClick={clickable ? () => onRowClick?.(row) : undefined}
                >
                  {columns.map((column) => (
                    <td
                      key={column.id}
                      className={[
                        'px-4 py-3 text-table text-text-mute-strong',
                        column.align === 'right' ? 'text-right' : 'text-left',
                      ].join(' ')}
                    >
                      {renderCell(column, row)}
                    </td>
                  ))}
                  {actions ? (
                    <td className="px-4 py-3 text-right" onClick={(event) => event.stopPropagation()}>
                      {actions(row) ?? (
                        <button
                          type="button"
                          className="inline-flex size-8 items-center justify-center rounded-pill text-text-mute hover:bg-panel-subtle hover:text-text"
                          aria-label="Row actions"
                        >
                          <MoreHorizontal size={17} aria-hidden="true" />
                        </button>
                      )}
                    </td>
                  ) : null}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
