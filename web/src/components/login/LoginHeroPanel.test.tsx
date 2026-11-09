import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { LoginHeroPanel } from './LoginHeroPanel';

describe('LoginHeroPanel', () => {
    it('explains the product without displaying telemetry or operational metrics', () => {
        const {container} = render(<LoginHeroPanel/>);
        expect(screen.getByText('ORCHIS')).toBeInTheDocument();
        expect(screen.getByRole('heading',{level:2})).toHaveTextContent('Know where you stand.');
        expect(screen.getByText('Understand the evidence')).toBeInTheDocument();
        expect(container.textContent).not.toMatch(/\d|uptime|latency|ID-JKT|assets indexed|all systems operational/i);
        expect(screen.queryByRole('tooltip')).toBeNull();
        expect(container.querySelectorAll('canvas,button,input')).toHaveLength(0);
    });
});
