const requiredHeadings = [
  'PR Type',
  'Summary',
  'Changes',
  'Test Plan',
  'Dotfiles Safety',
  'Contributor Checklist',
];

function validatePullRequest(pull) {
  const errors = [];
  const body = pull.body || '';
  const headings = [...body.matchAll(/^## (.+?)[ \t]*$/gm)].map((match) => match[1]);

  for (const heading of requiredHeadings) {
    const count = headings.filter((value) => value === heading).length;
    if (count !== 1) errors.push(`Expected exactly one "## ${heading}" heading; found ${count}.`);
  }

  const closingReferences = [...body.matchAll(/^Closes #([1-9]\d*)[ \t]*$/gim)];
  if (closingReferences.length !== 1) {
    errors.push(`Expected exactly one standalone "Closes #<issue-number>" line; found ${closingReferences.length}.`);
  }

  const typeLabels = (pull.labels || []).map((label) => label.name).filter((name) => name.startsWith('type:'));
  if (typeLabels.length !== 1) {
    errors.push(`Expected exactly one type:* label; found ${typeLabels.length}.`);
  }

  const typeSection = body.split(/^## PR Type[ \t]*$/m)[1]?.split(/^## /m)[0] || '';
  const checkedTypes = [...typeSection.matchAll(/^- \[[xX]\] .+?\(`(type:[\w-]+)`\)\s*$/gm)].map((match) => match[1]);
  if (checkedTypes.length !== 1) {
    errors.push(`Expected exactly one checked PR Type option; found ${checkedTypes.length}.`);
  } else if (typeLabels.length === 1 && checkedTypes[0] !== typeLabels[0]) {
    errors.push(`Checked PR Type ${checkedTypes[0]} does not match label ${typeLabels[0]}.`);
  }

  return errors;
}

module.exports = { validatePullRequest };
