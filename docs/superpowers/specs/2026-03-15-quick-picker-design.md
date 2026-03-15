# Quick Picker — Obsidian-Style File Finder

## Problem

Navigating to a document requires either browsing the sidebar tree or typing a full-text search query. Neither is fast for "I know roughly what it's called" — the most common navigation pattern.

## Solution

An Obsidian-style quick picker: a modal overlay triggered by `Cmd+O` that fuzzy-matches document paths from the local SwiftData cache. Instant, works offline.

## Requirements

- **Trigger**: `Cmd+O` (macOS), toolbar button (iOS)
- **Matching**: Client-side fuzzy match on document paths
- **Initial state**: Show recently accessed documents
- **Select**: Navigate to document
- **Create**: Allow creating new document at typed path when no exact match
- **iOS**: Full-screen sheet
- **macOS**: Sheet (~600x400)

## Architecture

### Data Flow

```
User types query
    ↓
FuzzyMatch against [CachedDocument] from SwiftData
    ↓
Ranked results with matched character indices
    ↓
QuickPickerView renders highlighted rows
    ↓
User selects → selectedDocumentId set → sheet dismissed
```

### Components

1. **FuzzyMatch** — pure function, scored character-order matching
2. **RecentDocument** — SwiftData model tracking last 50 accessed docs
3. **QuickPickerViewModel** — @Observable, holds query/results/selection state
4. **QuickPickerView** — search field + results list + keyboard hints
5. **QuickPickerRow** — path with fuzzy highlight + metadata badges

### Fuzzy Matching Scoring

- Characters must appear in order (not contiguous)
- +consecutive bonus for adjacent matches
- +boundary bonus for matches after `/`, `-`, `_`, space
- +start bonus for matches at beginning of path segments
- Case-insensitive
- Returns nil for no match

### Platform Behavior

| Aspect | macOS | iOS |
|--------|-------|-----|
| Trigger | Cmd+O | Toolbar button |
| Presentation | .sheet (600x400) | .sheet (full screen) |
| Navigation | Arrow keys + Enter | Tap |
| Create | Shift+Enter | "Create" row tap |
| Dismiss | Esc | Swipe down / X button |

### Integration Points

- `MainSplitView`: adds sheet, keyboard shortcut, recents tracking
- `KnowApp`: registers `RecentDocument` in ModelContainer
- `KnowService`: adds `createDocument(vaultId:path:)` method
