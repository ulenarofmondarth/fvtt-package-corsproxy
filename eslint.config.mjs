import globals from 'globals';
import pluginJs from '@eslint/js';
import prettier from 'eslint-config-prettier';

/**
 * Lints the repository's tooling (the commitlint and semantic-release configuration and their
 * tests). The proxy itself is Go, linted by golangci-lint (.golangci.yml).
 * @type {import('eslint').Linter.Config[]}
 */
export default [
  { ignores: ['coverage/**', 'dist/**', 'docs/_build/**', 'bench/**'] },
  { files: ['**/*.{js,mjs,cjs}'] },
  { languageOptions: { globals: globals.node } },
  pluginJs.configs.recommended,
  prettier,
];
