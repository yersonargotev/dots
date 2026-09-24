const assert = require('node:assert/strict');
const test = require('node:test');
const { validatePullRequest } = require('./pr-metadata.cjs');

const body = `Closes #525

## PR Type

- [x] Bug fix (\`type:bug\`)
- [ ] New feature (\`type:feature\`)

## Summary

- Guard package manager calls in tests.

## Changes

| File | Change |
|------|--------|
| \`test.go\` | Add stubs. |

## Test Plan

- [x] \`go test ./...\`

## Dotfiles Safety

- [x] Used a temporary home.

## Contributor Checklist

- [x] Linked the issue.
`;

function pull(changes = {}) {
  return { body, labels: [{ name: 'type:bug' }], ...changes };
}

test('accepts a filled template with a matching type label', () => {
  assert.deepEqual(validatePullRequest(pull()), []);
});

test('rejects missing or repeated required headings', () => {
  assert.match(validatePullRequest(pull({ body: body.replace('## Changes', '## Details') })).join('\n'), /Changes.*found 0/);
  assert.match(validatePullRequest(pull({ body: `${body}\n## Changes\n` })).join('\n'), /Changes.*found 2/);
});

test('requires exactly one closing issue reference', () => {
  assert.match(validatePullRequest(pull({ body: body.replace('Closes #525', 'Refs #525') })).join('\n'), /found 0/);
  assert.match(validatePullRequest(pull({ body: `${body}\nCloses #526\n` })).join('\n'), /found 2/);
});

test('requires one checked type matching one type label', () => {
  assert.match(validatePullRequest(pull({ labels: [] })).join('\n'), /type:\* label/);
  assert.match(validatePullRequest(pull({ labels: [{ name: 'type:bug' }, { name: 'type:chore' }] })).join('\n'), /found 2/);
  assert.match(validatePullRequest(pull({ labels: [{ name: 'type:chore' }] })).join('\n'), /does not match/);
  assert.match(validatePullRequest(pull({ body: body.replace('- [ ] New feature', '- [x] New feature') })).join('\n'), /checked PR Type.*found 2/);
});
