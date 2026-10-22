import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { parse } from 'yaml';

/**
 * @typedef {{ name?: string, uses?: string, with?: Record<string, unknown> }} Step
 * @typedef {{ env?: Record<string, string>, steps?: Step[] }} Job
 */

/** @type {{ jobs: Record<string, Job> }} */
const workflow = parse(readFileSync('.github/workflows/dev.yml', 'utf8'));
const tests = workflow.jobs['unit'];
const upload = tests?.steps?.find(step => step.uses?.startsWith('codecov/codecov-action@'));

describe('the Codecov upload', () => {
  it('is in the Tests job', () => {
    expect(upload).toBeDefined();
  });

  // A broken secret (a token pasted with something around it) once made every upload fail
  // without failing CI. Runs without the secret (Dependabot's, forks') must still pass.
  it('fails CI when a token is set and the upload fails, and only then', () => {
    expect(tests?.env?.['HAS_CODECOV_TOKEN']).toBe("${{ secrets.CODECOV_TOKEN != '' }}");
    expect(upload?.with?.['fail_ci_if_error']).toBe("${{ env.HAS_CODECOV_TOKEN == 'true' }}");
  });
});
