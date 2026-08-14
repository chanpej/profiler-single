import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Gauge } from 'lucide-react';
import { StatCard } from './StatCard';

describe('StatCard', () => {
  it('渲染标签、数值和副标题', () => {
    render(<StatCard icon={Gauge} label="平均 FPS" value="59.9" sub="45.0 ~ 60.0" tone="good" />);
    expect(screen.getByText('平均 FPS')).toBeInTheDocument();
    expect(screen.getByText('59.9')).toBeInTheDocument();
    expect(screen.getByText('45.0 ~ 60.0')).toBeInTheDocument();
    expect(screen.getByTestId('stat-card')).toHaveClass('tone-good');
  });

  it('无副标题时不渲染 sub 节点', () => {
    render(<StatCard icon={Gauge} label="内存" value="512MB" />);
    expect(screen.getByText('512MB')).toBeInTheDocument();
    expect(document.querySelector('.stat-card-sub')).toBeNull();
  });
});
