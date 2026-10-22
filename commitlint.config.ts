import { RuleConfigSeverity, type Rule, type UserConfig } from '@commitlint/types';

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

/** Fails subjects that start with a past-tense, gerund or third-person verb. */
const subjectImperative: Rule = ({ subject }) => {
  const firstWord = subject?.trim().split(/\s+/)[0] ?? '';
  return [
    !nonImperative.test(firstWord) || imperativeExceptions.has(firstWord.toLowerCase()),
    'subject must use the imperative mood ("add", not "added", "adds" or "adding")',
  ];
};

/** Dependabot writes long "bump x from y to z" messages; their format is not ours to police. */
const isDependabot = (message: string): boolean => /^(?:build|ci)\([\w-]+\): bump /.test(message);

const config: UserConfig = {
  extends: ['@commitlint/config-conventional'],
  ignores: [isDependabot],
  plugins: [{ rules: { 'subject-imperative': subjectImperative } }],
  rules: {
    'header-max-length': [RuleConfigSeverity.Error, 'always', 72],
    'scope-empty': [RuleConfigSeverity.Error, 'never'],
    'scope-case': [RuleConfigSeverity.Error, 'always', 'kebab-case'],
    'subject-imperative': [RuleConfigSeverity.Error, 'always'],
  },
};

export default config;
