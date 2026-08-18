import { useEffect, useRef, useState } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { FrameData } from '../types';
import { formatClock, formatCount, formatFps, formatGCAlloc, formatMs, fpsLevel } from '../lib/format';

interface Props {
  /** 当前可取的总帧数。 */
  totalCount: number;
  /** 按范围拉取帧数据（对接后端 Frames(offset, limit)）。 */
  fetchRange: (offset: number, limit: number) => Promise<FrameData[]>;
}

const ROW_HEIGHT = 30;
const FETCH_CHUNK = 200;

/**
 * 虚拟化帧数据表。只渲染可视区域的行，数据按块从后端懒加载，
 * 支撑十万级帧数据的流畅滚动。
 */
export function FrameTable({ totalCount, fetchRange }: Props) {
  const parentRef = useRef<HTMLDivElement | null>(null);
  const cacheRef = useRef<Map<number, FrameData>>(new Map());
  const pendingRef = useRef<Set<number>>(new Set());
  const [, setVersion] = useState(0);

  const virtualizer = useVirtualizer({
    count: totalCount,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 20,
  });

  const items = virtualizer.getVirtualItems();

  // 懒加载可视区域缺失的数据块。
  useEffect(() => {
    if (items.length === 0) return;
    const first = items[0].index;
    const last = items[items.length - 1].index;
    const chunks = new Set<number>();
    for (let i = first; i <= last; i++) {
      if (!cacheRef.current.has(i)) chunks.add(Math.floor(i / FETCH_CHUNK));
    }
    for (const chunk of chunks) {
      if (pendingRef.current.has(chunk)) continue;
      pendingRef.current.add(chunk);
      const offset = chunk * FETCH_CHUNK;
      fetchRange(offset, FETCH_CHUNK)
        .then((rows) => {
          rows.forEach((row, i) => cacheRef.current.set(offset + i, row));
          setVersion((v) => v + 1);
        })
        .finally(() => pendingRef.current.delete(chunk));
    }
  }, [items, fetchRange, totalCount]);

  // 会话被清空时重置缓存。
  useEffect(() => {
    if (totalCount === 0) {
      cacheRef.current.clear();
      setVersion((v) => v + 1);
    }
  }, [totalCount]);

  return (
    <div className="frame-table" data-testid="frame-table">
      <div className="frame-table-head" role="row">
        <span>帧号</span>
        <span>时间</span>
        <span>FPS</span>
        <span>CPU</span>
        <span>GPU</span>
        <span>GC</span>
        <span>内存</span>
        <span>DrawCall</span>
        <span>三角形</span>
      </div>
      <div ref={parentRef} className="frame-table-viewport">
        <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
          {items.map((item) => {
            const f = cacheRef.current.get(item.index);
            return (
              <div
                key={item.key}
                role="row"
                className="frame-table-row"
                style={{ transform: `translateY(${item.start}px)`, height: item.size }}
              >
                {f ? (
                  <>
                    <span className="mono dim">#{f.frameIndex}</span>
                    <span className="mono">{formatClock(f.timestampMs)}</span>
                    <span className={`mono fps-${fpsLevel(f.fps)}`}>{formatFps(f.fps)}</span>
                    <span className="mono">{formatMs(f.cpuTimeMs)}</span>
                    <span className="mono">{formatMs(f.gpuTimeMs)}</span>
                    <span className="mono">{formatGCAlloc(f.gcAllocKB)}</span>
                    <span className="mono">{f.usedMemMB.toFixed(0)}MB</span>
                    <span className="mono">{f.drawCalls}</span>
                    <span className="mono">{formatCount(f.triangles)}</span>
                  </>
                ) : (
                  <span className="mono dim">加载中…</span>
                )}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
