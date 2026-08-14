import type { LucideIcon } from 'lucide-react';

interface Props {
  icon: LucideIcon;
  label: string;
  value: string;
  sub?: string;
  tone?: 'good' | 'warn' | 'bad' | 'neutral';
}

/** 单个指标卡片。 */
export function StatCard({ icon: Icon, label, value, sub, tone = 'neutral' }: Props) {
  return (
    <div className={`stat-card tone-${tone}`} data-testid="stat-card">
      <div className="stat-card-icon">
        <Icon size={18} aria-hidden />
      </div>
      <div className="stat-card-body">
        <span className="stat-card-label">{label}</span>
        <span className="stat-card-value">{value}</span>
        {sub ? <span className="stat-card-sub">{sub}</span> : null}
      </div>
    </div>
  );
}
