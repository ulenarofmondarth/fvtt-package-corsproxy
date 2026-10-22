import { execFileSync } from 'node:child_process';
import { describe, expect, it } from 'vitest';

/**
 * The coverage gate in the Makefile compares a percentage with awk. Checked on the comparison
 * alone, without running the Go tests: `make -n` would only print the recipe.
 * @param {string} total
 * @param {string} min
 * @returns {boolean}
 */
function gate(total, min) {
  try {
    execFileSync('awk', ['-v', `total=${total}`, '-v', `min=${min}`, 'BEGIN { exit !(total >= min) }']);
    return true;
  } catch {
    return false;
  }
}

describe('the coverage gate', () => {
  it.each([
    ['89.0', '89', true],
    ['89.0', '89', true],
    ['88.9', '89', false],
    ['100.0', '89', true],
    ['9.5', '89', false],
  ])('passes %s%% against a minimum of %s%%: %s', (total, min, passes) => {
    expect(gate(total, min)).toBe(passes);
  });

  it('is the comparison the Makefile runs', () => {
    const recipe = execFileSync('make', ['-n', 'coverage'], { encoding: 'utf8' });
    expect(recipe).toContain(`awk -v total="$total" -v min="89" 'BEGIN { exit !(total >= min) }'`);
  });
});
