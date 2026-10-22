import { defineConfig } from 'vitest/config';

/**
 * Tests of the repository's own tooling: commitlint, semantic-release and the workflows. The
 * proxy's tests are Go (`go test ./...`).
 */
export default defineConfig({
  test: {
    include: ['tests/**/*.test.js'],
  },
});
