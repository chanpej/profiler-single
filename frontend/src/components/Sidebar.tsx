import { Activity, ListOrdered, Settings, Stethoscope } from 'lucide-react';
import type { ViewId } from '../types';

const items: { id: ViewId; icon: typeof Activity; label: string }[] = [
  { id: 'dashboard', icon: Activity, label: 'Profiler' },
  { id: 'frames', icon: ListOrdered, label: '帧数据' },
  { id: 'analysis', icon: Stethoscope, label: '诊断' },
  { id: 'settings', icon: Settings, label: '设置' },
];

interface Props {
  active: ViewId;
  onSelect: (view: ViewId) => void;
}

/** 左侧图标导航栏。 */
export function Sidebar({ active, onSelect }: Props) {
  return (
    <nav className="sidebar" aria-label="主导航">
      <div className="sidebar-logo" title="UnityPerfAgent">
        <Activity size={22} aria-hidden />
      </div>
      {items.map(({ id, icon: Icon, label }) => (
        <button
          key={id}
          type="button"
          className={`sidebar-btn ${active === id ? 'is-active' : ''}`}
          onClick={() => onSelect(id)}
          title={label}
          aria-label={label}
          aria-current={active === id ? 'page' : undefined}
        >
          <Icon size={20} aria-hidden />
        </button>
      ))}
    </nav>
  );
}
