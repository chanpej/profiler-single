import type { SeriesPoint } from './profiler';

export function moveChartSeries(order: string[], source: string, target: string): string[] {
  const from = order.indexOf(source);
  const to = order.indexOf(target);
  if (from < 0 || to < 0 || from === to) return order;
  const reordered = [...order];
  reordered.splice(from, 1);
  reordered.splice(to, 0, source);
  return reordered;
}

export function drawCpuAreas(
  context: CanvasRenderingContext2D,
  series: SeriesPoint[],
  frameIndices: number[],
  plot: { left: number; bottom: number; width: number; height: number; max: number },
) {
  if (!frameIndices.length) return;
  const step = plot.width / frameIndices.length;
  const scale = plot.height / plot.max;
  const yFor = (value: number) => plot.bottom - Math.min(value, plot.max) * scale;
  const xFor = (index: number) => plot.left + (index + 0.5) * step;
  const runs: { start: number; end: number }[] = [];
  let start = 0;
  for (let index = 1; index <= frameIndices.length; index++) {
    if (index < frameIndices.length && frameIndices[index] <= frameIndices[index - 1] + 1) continue;
    runs.push({ start, end: index - 1 });
    start = index;
  }

  let lower = Array<number>(frameIndices.length).fill(0);
  for (const category of series) {
    const upper = lower.map((value, index) => value + Math.max(0, category.values[index] || 0));
    context.fillStyle = category.color;
    for (const run of runs) {
      if (run.start === run.end) {
        const index = run.start;
        const top = yFor(upper[index]);
        context.fillRect(xFor(index) - step / 3, top, step * 2 / 3, yFor(lower[index]) - top);
        continue;
      }
      context.beginPath();
      context.moveTo(xFor(run.start), yFor(upper[run.start]));
      for (let index = run.start + 1; index <= run.end; index++) {
        context.lineTo(xFor(index), yFor(upper[index]));
      }
      for (let index = run.end; index >= run.start; index--) {
        context.lineTo(xFor(index), yFor(lower[index]));
      }
      context.closePath();
      context.fill();
    }
    lower = upper;
  }
}
