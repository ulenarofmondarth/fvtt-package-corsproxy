import { existsSync, readFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { describe, expect, it } from 'vitest';

/** @type {{ scripts: Record<string, string>, devDependencies: Record<string, string> }} */
const manifest = JSON.parse(readFileSync('package.json', 'utf8'));

/** Tools the scripts run that come from outside the project: Go's toolchain, make and Docker. */
const external = new Set(['go', 'gofmt', 'golangci-lint', 'make', 'docker', 'pnpm']);

/**
 * The program each command in a script runs, `a && b` and `a; b` included, after any leading
 * environment assignments (`NAME=$(id -u) docker …` runs docker).
 * @param {string} script
 * @returns {string[]}
 */
function commands(script) {
  return script.split(/&&|;|\|\|/).map(
    command =>
      command
        .replace(/\$\([^)]*\)/g, '')
        .trim()
        .split(/\s+/)
        .find(word => !/^\w+=/.test(word)) ?? '',
  );
}

describe('the package scripts', () => {
  // CI has only what the project installs: a script that works locally because a tool is
  // installed globally fails there.
  it.each(Object.entries(manifest.scripts))('%s runs only tools the project installs', (_name, script) => {
    const missing = commands(script).filter(
      command => !external.has(command) && !existsSync(`node_modules/.bin/${command}`),
    );
    expect(missing).toEqual([]);
  });
});

describe('the tooling', () => {
  // The proxy is Go; the tooling around it is plain JavaScript, with no compiler to keep.
  it('has no TypeScript sources', () => {
    const files = execFileSync('git', ['ls-files', '--cached', '--others', '--exclude-standard'], {
      encoding: 'utf8',
    })
      .split('\n')
      .filter(file => /\.(?:c|m)?ts$/.test(file) && !file.startsWith('bench/'));
    expect(files).toEqual([]);
  });

  it('does not depend on TypeScript', () => {
    const typescript = Object.keys(manifest.devDependencies).filter(name =>
      /^(?:typescript|typescript\d+|typescript-eslint|jiti|@types\/.*)$/.test(name),
    );
    expect(typescript).toEqual([]);
  });
});
