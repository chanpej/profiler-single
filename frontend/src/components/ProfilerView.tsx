import { useCallback, useEffect, useState } from 'react';
import type { FrameData, ProfilerModuleId } from '../types';
import { DEFAULT_ENABLED, PROFILER_MODULES } from '../lib/profiler';
import { ProfilerToolbar } from './ProfilerToolbar';
import { ProfilerChart } from './ProfilerChart';
import { ProfilerDetail } from './ProfilerDetail';

interface Props {
  frames: FrameData[];
  recording: boolean;
  onClear: () => void;
}

/** Unity Profiler 主视图：模块开关 + 堆叠时间轴 + 选中帧详情。 */
export function ProfilerView({ frames, recording, onClear }: Props) {
  const [enabled, setEnabled] = useState<ProfilerModuleId[]>(DEFAULT_ENABLED);
  const [active, setActive] = useState<ProfilerModuleId>('cpu');
  const [selected, setSelected] = useState(0);
  const [follow, setFollow] = useState(true);

  const index = frames.length === 0 ? 0 : follow ? frames.length - 1 : Math.min(selected, frames.length - 1);

  const clampSeek = useCallback((i: number) => {
    if (frames.length === 0) return;
    const next = Math.max(0, Math.min(frames.length - 1, i));
    setSelected(next);
    setFollow(next >= frames.length - 1);
  }, [frames.length]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft') {
        e.preventDefault();
        clampSeek(index - 1);
      } else if (e.key === 'ArrowRight') {
        e.preventDefault();
        clampSeek(index + 1);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [clampSeek, index]);

  const toggle = (id: ProfilerModuleId) => {
    setEnabled((cur) => {
      if (cur.includes(id)) {
        const next = cur.filter((x) => x !== id);
        return next.length === 0 ? cur : next;
      }
      return [...cur, id];
    });
    setActive(id);
  };

  const frame = frames[index] ?? null;
  const visible = PROFILER_MODULES.filter((m) => enabled.includes(m.id));

  return (
    <div className="profiler" data-testid="profiler-view">
      <ProfilerToolbar
        recording={recording}
        frame={frame}
        selectedIndex={frames.length === 0 ? 0 : index}
        total={frames.length}
        onClear={onClear}
        onPrev={() => clampSeek(index - 1)}
        onNext={() => clampSeek(index + 1)}
        onSeek={clampSeek}
        onCurrent={() => clampSeek(frames.length - 1)}
      />
      <div className="profiler-modules" role="group" aria-label="Profiler 模块">
        {PROFILER_MODULES.map((m) => (
          <label key={m.id} className={`profiler-mod ${enabled.includes(m.id) ? 'is-on' : ''}`}>
            <input
              type="checkbox"
              checked={enabled.includes(m.id)}
              onChange={() => toggle(m.id)}
              aria-label={m.label}
            />
            <span className="swatch" style={{ background: m.color }} />
            {m.label}
          </label>
        ))}
      </div>
      <div className="profiler-charts">
        {visible.map((m) => (
          <ProfilerChart
            key={m.id}
            module={m}
            frames={frames}
            selectedIndex={index}
            active={active === m.id}
            onSelectFrame={clampSeek}
            onActivate={() => setActive(m.id)}
          />
        ))}
      </div>
      <ProfilerDetail moduleId={active} frame={frame} />
    </div>
  );
}
