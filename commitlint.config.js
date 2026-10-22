/** @import { Rule, UserConfig } from '@commitlint/types' */

/** commitlint's severity for a rule that fails the commit. */
const ERROR = 2;

/**
 * Words that show a subject is not in the imperative mood ("added", "adding", "adds").
 * A heuristic: it catches the common cases, not every one.
 */
const nonImperative = /^(?:\w+(?:ed|ing)|adds|fixes|updates|removes|changes|makes|moves|renames|bumps|improves)$/i;

/** Imperative verbs that happen to end in "ed" or "ing". */
const imperativeExceptions = new Set([
  'bring',
  'embed',
  'exceed',
  'feed',
  'need',
  'proceed',
  'seed',
  'shed',
  'speed',
  'string',
]);

/**
 * Fails subjects that start with a past-tense, gerund or third-person verb.
 * @type {Rule}
 */
const subjectImperative = ({ subject }) => {
  const firstWord = subject?.trim().split(/\s+/)[0] ?? '';
  return [
    !nonImperative.test(firstWord) || imperativeExceptions.has(firstWord.toLowerCase()),
    'subject must use the imperative mood ("add", not "added", "adds" or "adding")',
  ];
};

/**
 * Dependabot writes long "bump x from y to z" messages; their format is not ours to police.
 * @param {string} message
 * @returns {boolean}
 */
const isDependabot = message => /^(?:build|ci)\([\w-]+\): bump /.test(message);

/** @type {UserConfig} */
const config = {
  extends: ['@commitlint/config-conventional'],
  ignores: [isDependabot],
  plugins: [{ rules: { 'subject-imperative': subjectImperative } }],
  rules: {
    'header-max-length': [ERROR, 'always', 72],
    'scope-empty': [ERROR, 'never'],
    'scope-case': [ERROR, 'always', 'kebab-case'],
    'subject-imperative': [ERROR, 'always'],
  },
};

export default config;
