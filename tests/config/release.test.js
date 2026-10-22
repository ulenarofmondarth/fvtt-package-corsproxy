import { execFileSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { parse } from 'yaml';
import config, { bumpVersionCmd, buildCmd } from '../../release.config.js';

/** @typedef {string | [string, Record<string, unknown>]} PluginSpec */

/**
 * The options a plugin is configured with, or `undefined` if it is not used.
 * @param {string} name
 * @returns {Record<string, unknown> | undefined}
 */
function pluginOptions(name) {
  /** @type {PluginSpec[]} */
  const plugins = config.plugins ?? [];
  const found = plugins.find(plugin => (Array.isArray(plugin) ? plugin[0] : plugin) === name);
  if (found === undefined) return undefined;
  return Array.isArray(found) ? found[1] : {};
}

/**
 * @typedef {{ jobs: Record<string, { if?: string, outputs?: Record<string, string>, steps?: { with?: Record<string, string> }[] }> }} Workflow
 */

/** @type {Workflow} */
const workflow = parse(readFileSync('.github/workflows/release.yml', 'utf8'));

describe('the release configuration', () => {
  it('releases from main only, tagged v<version> as upstream was', () => {
    expect(config.branches).toEqual(['main']);
    expect(config.tagFormat).toBe('v${version}');
  });

  it('does not use @semantic-release/npm, which runs npm', () => {
    expect(pluginOptions('@semantic-release/npm')).toBeUndefined();
  });

  it('sets the version before building, so the binaries carry it', () => {
    expect(pluginOptions('@semantic-release/exec')?.['prepareCmd']).toBe(`${bumpVersionCmd} && ${buildCmd}`);
    expect(buildCmd).toBe('make dist VERSION=v${nextRelease.version}');
  });

  it('commits the changelog and the version it sets', () => {
    expect(pluginOptions('@semantic-release/git')?.['assets']).toEqual(['CHANGELOG.md', 'package.json']);
  });

  it('attaches the binaries and their checksums that make dist builds', () => {
    const assets = /** @type {{ path: string }[]} */ (pluginOptions('@semantic-release/github')?.['assets']);
    expect(assets.map(({ path }) => path)).toEqual(['dist/cors-proxy-*', 'dist/checksums.txt']);
    const makefile = readFileSync('Makefile', 'utf8');
    expect(makefile).toContain('-o "dist/cors-proxy-$$os-$$arch$$ext"');
    expect(makefile).toContain('sha256sum cors-proxy-* > checksums.txt');
  });
});

describe('the release workflow', () => {
  it('hands the tag semantic-release writes to the jobs that need it', () => {
    expect(pluginOptions('@semantic-release/exec')?.['successCmd']).toBe(
      'echo "tag=${nextRelease.gitTag}" >> "$GITHUB_OUTPUT"',
    );
    expect(workflow.jobs['release']?.outputs).toEqual({ tag: '${{ steps.release.outputs.tag }}' });
    for (const job of ['image', 'sync-dev']) {
      expect(workflow.jobs[job]?.if).toBe("needs.release.outputs.tag != ''");
    }
  });

  it('builds the image from the release tag, stamped with it', () => {
    const steps = workflow.jobs['image']?.steps ?? [];
    expect(steps[0]?.with?.['ref']).toBe('${{ needs.release.outputs.tag }}');
    expect(steps.at(-1)?.with?.['build-args']).toBe('VERSION=${{ needs.release.outputs.tag }}');
  });
});

describe('the version bump', () => {
  /** @type {string | undefined} */
  let dir;

  afterEach(() => {
    if (dir) rmSync(dir, { recursive: true, force: true });
    dir = undefined;
  });

  it('sets the version in package.json and changes nothing else, offline', () => {
    dir = mkdtempSync(join(tmpdir(), 'corsproxy-release-'));
    const files = ['package.json', 'pnpm-workspace.yaml', 'pnpm-lock.yaml'].filter(file => existsSync(file));
    for (const file of files) copyFileSync(file, join(dir, file));
    const workspace = dir;
    const contents = () => Object.fromEntries(files.map(file => [file, readFileSync(join(workspace, file), 'utf8')]));
    const before = contents();
    const cache = join(dir, 'cache');

    execFileSync('sh', ['-c', bumpVersionCmd.replaceAll('${nextRelease.version}', '9.8.7')], {
      cwd: dir,
      stdio: 'pipe',
      // An empty pnpm cache: anything written to it was fetched from the registry.
      env: { ...process.env, XDG_CACHE_HOME: cache, npm_config_cache_dir: join(cache, 'pnpm') },
    });

    expect(contents()).toEqual({
      ...before,
      'package.json': (before['package.json'] ?? '').replace(/"version": "[^"]*"/, '"version": "9.8.7"'),
    });
    expect(existsSync(cache)).toBe(false);
  });
});
