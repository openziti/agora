import { ChevronLeft, ChevronRight } from 'lucide-react';

export type PaginationProps = {
  totalItems: number;
  itemsPerPage: number;
  currentPage: number;
  onPageChange: (page: number) => void;
  onItemsPerPageChange: (count: number) => void;
};

const PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

export function Pagination({
  totalItems,
  itemsPerPage,
  currentPage,
  onPageChange,
  onItemsPerPageChange,
}: PaginationProps) {
  const totalPages = Math.max(1, Math.ceil(totalItems / itemsPerPage));
  const from = totalItems === 0 ? 0 : (currentPage - 1) * itemsPerPage + 1;
  const to = Math.min(currentPage * itemsPerPage, totalItems);

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3 text-table text-text-mute">
      <span>
        Showing {from}–{to} of {totalItems}
      </span>
      <div className="flex items-center gap-3">
        <select
          value={itemsPerPage}
          onChange={(e) => {
            onItemsPerPageChange(Number(e.target.value));
            onPageChange(1);
          }}
          className="h-8 rounded-pill border border-border bg-panel px-2 text-table text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
        >
          {PAGE_SIZE_OPTIONS.map((size) => (
            <option key={size} value={size}>
              {size} per page
            </option>
          ))}
        </select>
        <div className="flex items-center gap-1">
          <button
            type="button"
            disabled={currentPage <= 1}
            onClick={() => onPageChange(currentPage - 1)}
            className="inline-flex size-8 items-center justify-center rounded-pill border border-border bg-panel text-text-mute-strong hover:bg-panel-subtle disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            aria-label="Previous page"
          >
            <ChevronLeft size={14} aria-hidden="true" />
          </button>
          <span className="px-2 text-table text-text-mute-strong sm:hidden">
            {currentPage} / {totalPages}
          </span>
          <span className="hidden sm:contents">
            {buildPageRange(currentPage, totalPages).map((item, i) =>
              item === null ? (
                <span key={`ellipsis-${i}`} className="px-1 text-text-mute">
                  …
                </span>
              ) : (
                <button
                  key={item}
                  type="button"
                  onClick={() => onPageChange(item)}
                  aria-current={item === currentPage ? 'page' : undefined}
                  className={[
                    'inline-flex size-8 items-center justify-center rounded-pill border text-table font-medium focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
                    item === currentPage
                      ? 'border-brand-agora bg-brand-agora/10 text-brand-agora'
                      : 'border-border bg-panel text-text-mute-strong hover:bg-panel-subtle',
                  ].join(' ')}
                >
                  {item}
                </button>
              ),
            )}
          </span>
          <button
            type="button"
            disabled={currentPage >= totalPages}
            onClick={() => onPageChange(currentPage + 1)}
            className="inline-flex size-8 items-center justify-center rounded-pill border border-border bg-panel text-text-mute-strong hover:bg-panel-subtle disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            aria-label="Next page"
          >
            <ChevronRight size={14} aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>
  );
}

function buildPageRange(current: number, total: number): (number | null)[] {
  if (total <= 7) {
    return Array.from({ length: total }, (_, i) => i + 1);
  }
  const pages: (number | null)[] = [1];
  if (current > 3) pages.push(null);
  for (let i = Math.max(2, current - 1); i <= Math.min(total - 1, current + 1); i++) {
    pages.push(i);
  }
  if (current < total - 2) pages.push(null);
  pages.push(total);
  return pages;
}
