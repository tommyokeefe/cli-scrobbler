# Working Preferences and Standards

## Code Style

- **Formatter:** `gofmt` — no exceptions.
- **Linter:** `golangci-lint` defaults. No custom rules unless there's a specific reason.
- **Error handling:** Standard Go idioms (`fmt.Errorf` with `%w` for wrapping, `errors.New` for leaf errors). No strong opinion beyond what the linter enforces.
- **Naming and layout:** Follow standard Go conventions. No strong opinions on package splits — discuss before reorganising.
- **Comment density:** Don't over-comment. Comments should explain *why*, not *what*.

## Testing Standards

- **Coverage:** Maintain decent coverage — enough that users and contributors can trust the project.
- **Quality over quantity:** Tests must cover real functionality. Happy paths are not enough; edge cases and error paths matter.
- **Style:** Table-driven tests are idiomatic and preferred for multi-case scenarios. Mocks via interfaces where appropriate.
- **Don't test the obvious:** Avoid tests that only verify that code compiles or that a mock was called.

## AI Collaboration Preferences

- **Ask before opinionated changes.** If a change involves architectural decisions, naming, package reorganisation, or anything that could reasonably go another way — ask first rather than assuming.
- **Explain your reasoning.** When communicating about decisions or tradeoffs, err on the side of providing context and explanation rather than being terse.
- **Minimal, targeted changes.** Make the smallest change that solves the problem. Don't refactor surrounding code unless asked.
- **No unsolicited improvements.** Don't add features, abstractions, or error handling for scenarios outside the stated task.

## Communication Preferences

- Be direct and technical.
- Provide context when making recommendations — especially for anything architectural or opinionated.
- Raise concerns or alternatives before implementing, not after.
