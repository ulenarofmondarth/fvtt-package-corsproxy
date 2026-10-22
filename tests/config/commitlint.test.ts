import { describe, expect, it } from 'vitest';
import config from '../../commitlint.config.js';

type Rule = (parsed: { subject: string | null }) => [boolean, string?];

const rule = config.plugins?.[0];
const subjectImperative = (typeof rule === 'object' ? rule.rules['subject-imperative'] : undefined) as Rule | undefined;
const isIgnored = (message: string): boolean => config.ignores?.some(ignore => ignore(message)) ?? false;

describe('commitlint configuration', () => {
  it.each(['add the release workflow', 'fix a crash', 'embed the handler', 'need a test', 'speed up the build'])(
    'accepts the imperative subject %j',
    subject => {
      expect(subjectImperative?.({ subject })[0]).toBe(true);
    },
  );

  it.each(['added the workflow', 'adding the workflow', 'adds the workflow', 'fixes a crash', 'updated deps'])(
    'rejects the subject %j',
    subject => {
      expect(subjectImperative?.({ subject })[0]).toBe(false);
    },
  );

  it('treats a missing subject as acceptable (subject-empty reports it)', () => {
    expect(subjectImperative?.({ subject: null })[0]).toBe(true);
  });

  it('ignores Dependabot version bumps', () => {
    expect(isIgnored('build(deps-dev): bump vitest from 4.0.0 to 4.1.0 in the vitest group')).toBe(true);
    expect(isIgnored('build(deps): add a new dependency')).toBe(false);
  });

  it('requires a scope and limits the header to 72 characters', () => {
    expect(config.rules?.['scope-empty']).toEqual([2, 'never']);
    expect(config.rules?.['header-max-length']).toEqual([2, 'always', 72]);
  });
});
