import { useCallback, useEffect, useRef, useState } from 'react';
import type { MouseEvent as ReactMouseEvent } from 'react';

type UseResizableDrawerOptions = {
  defaultWidth: number;
  minWidth: number;
  maxWidth: number;
};

export function useResizableDrawer({ defaultWidth, minWidth, maxWidth }: UseResizableDrawerOptions) {
  const [width, setWidth] = useState(defaultWidth);
  const isDragging = useRef(false);

  const onMouseDown = useCallback(
    (e: ReactMouseEvent) => {
      e.preventDefault();
      isDragging.current = true;

      const handleMouseMove = (ev: MouseEvent) => {
        if (!isDragging.current) return;
        setWidth(Math.max(minWidth, Math.min(maxWidth, window.innerWidth - ev.clientX)));
      };

      const handleMouseUp = () => {
        isDragging.current = false;
        document.removeEventListener('mousemove', handleMouseMove);
        document.removeEventListener('mouseup', handleMouseUp);
      };

      document.addEventListener('mousemove', handleMouseMove);
      document.addEventListener('mouseup', handleMouseUp);
    },
    [minWidth, maxWidth],
  );

  useEffect(
    () => () => {
      isDragging.current = false;
    },
    [],
  );

  return { width, dragHandleProps: { onMouseDown } };
}
