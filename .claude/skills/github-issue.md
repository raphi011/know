---
name: github-issue
description: |
  Manage GitHub issues for the knowhow project. Use when the user wants to create, list, search, view, update, or close issues, or when discussing bugs/features that should become issues. Triggers on: "create issue", "file a bug", "feature request", "list issues", "close issue", "open issue", "github issue", "report bug", "add to project board", or when the user says something like "this should be an issue" or "let's track this".
---

# GitHub Issue Management

You are managing GitHub issues for the `knowhow` repository. All operations use the `gh` CLI.

## 1. Detect Action

From the conversation context, determine the action:

| Action | Signals |
|--------|---------|
| **create** | "create issue", "file a bug", "feature request", "this should be an issue", discussing a bug or feature |
| **list** | "list issues", "show issues", "what's open" |
| **search** | "find issue about...", "search issues" |
| **view** | "show issue #N", "what's in issue N" |
| **update** | "add label to #N", "assign #N", "rename issue", "comment on #N" |
| **close** | "close #N", "resolve #N" |
| **add-to-project** | "add #N to project board" |

If the action is ambiguous, ask the user.

## 2. Create Flow

### Auto-detect issue type

- **Bug**: conversation mentions broken behavior, errors, unexpected results, regressions
- **Feature/Enhancement**: conversation mentions new functionality, improvements, "it would be nice if"

### Draft the issue

**For bugs**, use the bug report template:
```markdown
## Bug
<describe what's broken, derived from conversation>

## Steps to Reproduce
1. <steps from conversation context>

## Expected vs Actual
<what should happen vs what happens>

## Context
<version, OS, logs — include any error messages from conversation>
```

**For features**, use the feature request template:
```markdown
## Problem
<what problem does this solve, from conversation>

## Proposed Solution
<how should it work, from conversation>

## Implementation Notes
<any architectural notes, key files mentioned in conversation>

## Acceptance Criteria
- [ ] <criteria derived from conversation>
```

### Confirm with user

Present the drafted issue (title, body, labels) and ask for confirmation using AskUserQuestion before creating. Show:
- **Title**: `<drafted title>`
- **Type**: Bug / Feature
- **Labels**: `bug` or `enhancement`
- **Body**: (the full drafted body)

Ask: "Create this issue? You can suggest edits or approve."

### Execute on approval

```bash
# Create the issue
gh issue create --title "<title>" --body "<body>" --label "<label>"

# Add to project board (Knowhow project #4)
gh project item-add 4 --owner raphi011 --url <issue-url>
```

Report back with the issue URL.

## 3. List Flow

```bash
# Basic list
gh issue list

# With filters (use as applicable)
gh issue list --label "<label>"
gh issue list --assignee "<user>"
gh issue list --state closed
gh issue list --search "<query>"
```

Present results in a readable table format.

## 4. Search Flow

```bash
gh issue list --search "<user's search query>"
```

## 5. View Flow

```bash
gh issue view <number>
```

Present the issue details clearly.

## 6. Update Flow

```bash
# Edit title
gh issue edit <number> --title "<new title>"

# Add/remove labels
gh issue edit <number> --add-label "<label>"
gh issue edit <number> --remove-label "<label>"

# Add assignee
gh issue edit <number> --add-assignee "<user>"

# Add comment
gh issue comment <number> --body "<comment>"
```

## 7. Close Flow

```bash
# Close without comment
gh issue close <number>

# Close with comment
gh issue close <number> --comment "<reason>"
```

## 8. Add to Project Board

```bash
gh project item-add 4 --owner raphi011 --url <issue-url>
```

The issue URL format is `https://github.com/raphi011/knowhow/issues/<number>`.

## Available Labels

`bug`, `enhancement`, `documentation`, `duplicate`, `good first issue`, `help wanted`, `invalid`, `question`, `wontfix`

## Rules

- **Always confirm** before creating, closing, or making destructive changes
- **Always add to project board** after creating an issue
- **Use conversation context** to fill in as much detail as possible when creating
- **Be concise** in issue titles — under 70 characters
- Keep issue bodies focused and actionable
