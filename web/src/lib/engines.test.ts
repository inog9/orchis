import { expect, it } from 'vitest';
import { agentDisplayName, engineSetLabel, toolsFor } from './engines';

it.each(['Hackbot', 'hackbot', 'HACKBOT'])('displays the current name for saved %s labels', name => {
  expect(agentDisplayName(`${name} could not complete this audit.`)).toBe('orchis-redteam-agent could not complete this audit.');
});

it('preserves current names and the tool identifiers used by existing workers', () => {
  expect(agentDisplayName('orchis-redteam-agent + orchis-defense-agent')).toBe('orchis-redteam-agent + orchis-defense-agent');
  expect(toolsFor('web')).toEqual(['nmap', 'nuclei', 'hackbot']);
  expect(engineSetLabel(toolsFor('web'))).toBe('orchis-redteam-agent');
});
