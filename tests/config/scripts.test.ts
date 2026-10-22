import { existsSync, readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

/**
 * Each tool a script runs must come from the project's own dependencies: CI has nothing else, so
 * a script that works only because a tool is installed globally fails there.
 */

const { scripts } = JSON.parse(readFileSync('package.json', 'utf8')) as { scripts: Record<string, string> };

describe('the type check scripts', () => {
  it.each([
    ['typecheck', 'tsc'],
    ['typecheck:fallback', 'tsc6'],
  ])('%s runs %s, which the project installs', (script, bin) => {
    expect(scripts[script]).toBe(`${bin} --noEmit`);
    expect(existsSync(`node_modules/.bin/${bin}`)).toBe(true);
  });
});
