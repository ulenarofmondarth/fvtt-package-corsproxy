import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

/**
 * A tag such as `@v7` can be moved by whoever controls the action's repository, so a
 * compromised third-party action would run in our workflows with their tokens (CodeQL
 * actions/unpinned-tag). Third-party actions are pinned to a full commit SHA, with the release in
 * a trailing comment that Dependabot keeps up to date. GitHub's own actions (actions/*, github/*)
 * may use tags.
 */

const trustedOwners = new Set(['actions', 'github']);

/** Every workflow and composite action in .github. */
function workflowFiles(): string[] {
  const workflows = readdirSync('.github/workflows')
    .filter(name => /\.ya?ml$/.test(name))
    .map(name => join('.github/workflows', name));
  const actions = readdirSync('.github/actions').map(name => join('.github/actions', name, 'action.yml'));
  return [...workflows, ...actions];
}

/** The `uses:` references to actions in other repositories, with where they are. */
function thirdPartyUses(): { where: string; ref: string; comment: string }[] {
  return workflowFiles().flatMap(file =>
    readFileSync(file, 'utf8')
      .split('\n')
      .flatMap((line, index) => {
        const match = /^\s*(?:-\s+)?uses:\s*([^\s#]+)\s*(?:#\s*(.*))?$/.exec(line);
        const ref = match?.[1];
        if (ref === undefined || ref.startsWith('./')) return [];
        if (trustedOwners.has(ref.split('/')[0] ?? '')) return [];
        return [{ where: `${file}:${String(index + 1)}`, ref, comment: match?.[2] ?? '' }];
      }),
  );
}

describe('third-party actions', () => {
  const uses = thirdPartyUses();

  it('are found', () => {
    expect(uses.length).toBeGreaterThan(0);
  });

  it('are pinned to a full commit SHA', () => {
    const unpinned = uses.filter(({ ref }) => !/@[0-9a-f]{40}$/.test(ref)).map(({ where, ref }) => `${where} ${ref}`);
    expect(unpinned).toEqual([]);
  });

  it('name the pinned release in a comment', () => {
    const unnamed = uses.filter(({ comment }) => !/^v\d+\.\d+\.\d+$/.test(comment)).map(({ where }) => where);
    expect(unnamed).toEqual([]);
  });
});
