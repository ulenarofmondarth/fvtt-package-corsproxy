/**
 * semantic-release, run by the release workflow on pushes to `main` (`pnpm release`).
 *
 * The version follows strict SemVer derived from Conventional Commits: `fix` -> patch,
 * `feat` -> minor, `BREAKING CHANGE:` / `!` -> major. Releases are tagged `v<version>`, carrying on
 * from upstream's `v1.2.6`. Day-to-day work lands on `dev`; only `main` is released.
 *
 * The version is stamped into the binaries at build time (`make dist`, `-X main.version`), and
 * kept in package.json for the documentation. The release workflow builds the container image
 * from the tag afterwards.
 */

/** Sets the version in package.json. pnpm, not `npm version`: devEngines makes npm refuse to run here. */
export const bumpVersionCmd = 'pnpm pkg set version=${nextRelease.version}';

/** Cross-compiles the release binaries and their checksums into dist/. */
export const buildCmd = 'make dist VERSION=v${nextRelease.version}';

/** @type {import('semantic-release').GlobalConfig} */
export default {
  branches: ['main'],
  tagFormat: 'v${version}',
  plugins: [
    ['@semantic-release/commit-analyzer', { preset: 'conventionalcommits' }],
    ['@semantic-release/release-notes-generator', { preset: 'conventionalcommits' }],
    ['@semantic-release/changelog', { changelogFile: 'CHANGELOG.md', changelogTitle: '# Changelog' }],
    [
      '@semantic-release/exec',
      {
        // Sets the version first, so what is built carries it.
        prepareCmd: `${bumpVersionCmd} && ${buildCmd}`,
        // Hands the release's tag to the later jobs of the release workflow.
        successCmd: 'echo "tag=${nextRelease.gitTag}" >> "$GITHUB_OUTPUT"',
      },
    ],
    [
      '@semantic-release/git',
      {
        assets: ['CHANGELOG.md', 'package.json'],
        message: 'chore(release): ${nextRelease.gitTag} [skip ci]\n\n${nextRelease.notes}',
      },
    ],
    [
      '@semantic-release/github',
      {
        assets: [
          { path: 'dist/cors-proxy-*', label: '' },
          { path: 'dist/checksums.txt', label: 'SHA-256 checksums' },
        ],
      },
    ],
  ],
};
