# Contribution Guide

Thank you for contributing! This guide describes our conventions for branches, commits, pull requests, and Go development.

## Before You Start

Before beginning work:

1. Check existing issues and pull requests to avoid duplicating work.
2. For non-trivial changes, create or comment on an issue to discuss the proposed solution.
3. Make sure your local branch is up to date with the default branch.
4. Keep changes focused on one issue or feature whenever possible.

## Branches

Create a separate branch for each feature, bug fix, or maintenance task. Do not work directly on the default branch.

Branch names should be descriptive, lowercase, and use kebab-case:

- Clearly describe the work
- Be reasonably concise
- Use lowercase letters and hyphens
- Avoid vague names such as `updates`, `changes`, or `misc`
- Not include unrelated work

Use one of these prefixes where appropriate:

- `feature/`: New functionality
- `fix/`: Bug fixes
- `docs/`: Documentation-only changes
- `refactor/`: Code changes that do not alter behavior
- `test/`: Adding or updating tests
- `chore/`: Routine maintenance, tooling, or dependency updates

Include the issue number when relevant:

```text
feature/123-add-user-search
fix/456-handle-empty-api-response
docs/update-deployment-guide
refactor/simplify-auth
```

Create branches from the latest version of the default branch:

```bash
git checkout main
git pull origin main
git checkout -b feature/123-add-user-search
```

### Keeping Branches Up to Date

Before opening a pull request, update your branch with the latest changes from the default branch:

```bash
git fetch origin
git rebase origin/main
```

Resolve conflicts locally, rerun the formatting and test commands, and push the updated branch:

```bash
git push --force-with-lease
```

`--force-with-lease` updates the remote branch only if it has not changed since your last fetch. This helps prevent accidentally overwriting someone else's work.

Do not force-push to shared branches or branches used by other contributors.

## Commits

Write clear, focused commits. Each commit should represent one logical change and should be easy to review or revert. Small, focused commits are preferred over large, complex ones.

Use imperative language and the following format:

```text
<type>: <short description>
```

Examples:

```text
feat: add user search endpoint
fix: handle empty API response
docs: update deployment instructions
test: add coverage for invalid search input
refactor: simplify auth flow
chore: update dependencies
```

Recommended commit types:

- `feat`: New functionality
- `fix`: Bug fixes
- `docs`: Documentation-only changes
- `test`: Test changes
- `refactor`: Changes that do not alter behavior
- `chore`: Routine maintenance, tooling, or dependency updates
- `build`: Build system or dependency changes, such as Dockerfile changes
- `ci`: Continuous integration changes

Commits should:

- Be small and logically focused
- Avoid mixing unrelated changes
- Avoid committing generated files, build artifacts, credentials, or local configuration
- Include tests when appropriate
- Use a commit body to explain non-obvious decisions
- Be reviewed before being pushed

Review staged changes before committing:

```bash
git diff --staged
git status
```

Do not commit secrets, API keys, passwords, or other sensitive information. Add files containing local configuration to `.gitignore` when appropriate.

## Go Development

Run these commands from the repository root before opening a pull request.

### Format Code

```bash
gofmt -w .
```

Go code should be formatted with `gofmt` before it is committed. Avoid mixing formatting changes with unrelated functional changes.

### Run Tests

Run all tests in the repository:

```bash
go test ./...
```

The `./...` pattern tells Go to test the current package and all packages below it.

Other useful variations:

```bash
go test -v ./...                        # verbose output, shows each test as it runs
go test ./path/to/package                # test a specific package
go test ./path/to/package -run '^TestName$'  # run one specific test
go test -race ./...                      # check for unsafe concurrent access (slower)
```

Run the race detector whenever you change concurrent code.

New or modified behavior should include appropriate tests. Tests should be deterministic, cover normal operation, invalid input, error conditions, and important edge cases, and should not depend on external services unless explicitly required.

### Check Test Coverage

```bash
go test -cover ./...
```

For a detailed HTML report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Don't commit the generated `coverage.out` file unless the project specifically requires it.

### Run `go vet`

```bash
go vet ./...
```

`go vet` flags suspicious or likely-incorrect code, such as invalid formatting directives, unreachable code, and some incorrect API usages. Pull requests should not introduce new `go vet` warnings.

### Tidy Dependencies

```bash
go mod tidy
```

`go mod tidy` adds missing dependencies to `go.mod`, removes unused ones, and updates `go.sum`. Review the resulting changes before committing:

```bash
git diff -- go.mod go.sum
```

Only commit changes to `go.mod` or `go.sum` when they are required by your branch.

### Run `golangci-lint`

If the project uses a linter, run it before requesting review:

```bash
golangci-lint run
```

This checks for unused code, error-handling problems, suspicious constructs, style violations, common bugs, and ineffective assignments. Fix reported issues before opening a pull request. If a finding is intentional and can't be fixed, document the reason in the pull request.

### Recommended Pre-PR Checklist

```bash
gofmt -w .
go test ./...
go test -race ./...
go test -cover ./...
go vet ./...
go mod tidy
golangci-lint run
```

After running `go mod tidy`, review the resulting changes (`git status`, `git diff`) to confirm that formatting changes, dependency changes, generated files, and test artifacts are intentional before committing.

## Pull Requests

Open a pull request when your work is ready for review.

Every pull request must:

- Reference the issue it addresses
- Include a clear summary of the change
- Explain why the change is needed
- Describe the tests and checks that were run
- Identify risks, limitations, or follow-up work
- Include screenshots, recordings, or example output when relevant
- Keep the scope limited to the associated issue

Reference issues using GitHub keywords when the pull request should automatically close the issue:

```text
Closes #123
Fixes #456
Resolves #789
```

Use a descriptive pull request title, following the same convention as commits:

```text
feat: add user search endpoint
fix: handle empty API responses
docs: update deployment instructions
```

### Pull Request Description Template

```markdown
## Summary

Describe what changed and why.

## Related Issue

Closes #123

## Changes

- Add the user search endpoint
- Validate search parameters
- Add tests for successful and unsuccessful searches

## Testing

Describe the tests and checks you ran.

- `go test ./...`
- `go vet ./...`
- `gofmt`
- `golangci-lint run`

## Screenshots or Example Output

Add screenshots, recordings, logs, or example output if applicable.

## Risks and Follow-Up Work

Describe any known risks, limitations, or follow-up tasks.

## Checklist

- [ ] I have referenced the related issue.
- [ ] I have tested these changes locally.
- [ ] I have run `gofmt`.
- [ ] I have run the relevant Go tests and checks.
- [ ] I have added or updated tests where appropriate.
- [ ] I have updated documentation where necessary.
- [ ] I have reviewed my own changes.
- [ ] I have confirmed that no secrets or sensitive information are included.
- [ ] I have kept this pull request focused on one issue or feature.
```

## Code Review

Pull request authors should:

- Respond to review comments constructively.
- Ask for clarification when feedback is unclear.
- Keep discussions focused on the code and requirements.
- Address review comments before requesting another review.
- Re-request review after making substantial changes.

Reviewers should consider:

- Correctness and maintainability
- Test coverage
- Error handling
- Security implications
- Performance implications
- Compatibility with existing behavior
- Documentation and user impact

## Reducing Merge Conflicts

To reduce merge conflicts and keep reviews manageable:

- Keep pull requests small and focused.
- Avoid reformatting unrelated files.
- Do not combine dependency upgrades with feature work unless necessary.
- Coordinate with other contributors before editing the same large or frequently changed files.
- Regularly update long-running branches from the default branch.
- Avoid committing generated files unless they are required.
- Separate mechanical changes, such as renaming or formatting, from functional changes.
- Resolve conflicts carefully and rerun all relevant tests afterward.
- Avoid changing shared configuration or public APIs without discussing the change first.

## Merging

Before merging, confirm that:

- Required reviews are complete.
- Automated checks are passing.
- The related issue is referenced.
- Merge conflicts have been resolved.
- Tests and documentation are up to date.
- No unrelated changes are included.