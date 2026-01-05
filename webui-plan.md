# Webui Plan for memcp

Goal: Create a web-based user interface for **memcp** - a persistent memory system for AI agents.

---

## What is memcp?

memcp is a knowledge graph backend that gives AI agents persistent memory across sessions. It stores information as **entities** connected by **relations**, enabling semantic search, graph traversal, and contradiction detection.

### Data Model

#### Entity (Memory Unit)
```typescript
interface Entity {
  id: string;           // e.g. "user_pref_dark_mode", "project_memcp"
  content: string;      // The actual fact/memory text
  type: string;         // Category: "concept", "preference", "fact", "decision"
  labels: string[];     // Tags: ["project", "python", "ai"]
  confidence: number;   // 0.0 to 1.0 reliability score
  source: string;       // Origin/provenance
  created: datetime;
  accessed: datetime;
  access_count: number;
  decay_weight: number; // Temporal relevance (decreases with age)
}
```

#### Relation (Graph Edge)
```typescript
interface Relation {
  from: string;    // Source entity ID
  to: string;      // Target entity ID
  type: string;    // e.g. "prefers", "uses", "relates_to", "fixed_by"
  weight?: number; // Relationship strength
}
```

### Available API Operations

| Operation | Description |
|-----------|-------------|
| `search(query, labels?, semantic_weight?)` | Hybrid semantic + keyword search |
| `get_entity(id)` | Fetch single entity by ID |
| `list_labels()` | Get all tags in the system |
| `traverse(start, depth, relation_types?)` | Graph traversal from entity |
| `find_path(from, to)` | Shortest path between entities |
| `remember(entities, relations)` | Store new memories |
| `forget(id)` | Delete entity and its relations |
| `reflect()` | Maintenance: decay old memories, find duplicates |
| `check_contradictions()` | Detect conflicting information |

### Stats Resource
```typescript
interface MemoryStats {
  total_entities: number;
  total_relations: number;
  labels: string[];
  label_counts: Record<string, number>;
}
```

---

## UI Features

### 1. Dashboard / Stats Overview
**Purpose:** Landing page showing memory health at a glance.

**Display:**
- Total entities count
- Total relations count
- Label distribution (pie/bar chart)
- Recent activity (last accessed entities)
- Memory health indicators (stale memories, potential duplicates)

---

### 2. Search Page
**Purpose:** Find memories using hybrid semantic + keyword search.

**Components:**
- Search input with query text
- Filter chips for labels (multi-select from `list_labels()`)
- Semantic weight slider (0.0 keyword-only ↔ 1.0 pure semantic)
- Results list showing:
  - Entity content (highlight matching terms)
  - Labels as colored chips
  - Confidence badge
  - Last accessed date
  - Access count
- Click result → navigate to Detail View

---

### 3. Graph View
**Purpose:** Visualize the knowledge graph and entity relationships.

**Components:**
- Interactive node-link diagram (force-directed or hierarchical)
- Nodes = entities, colored by type or label
- Edges = relations, labeled with relationship type
- Node size = access_count or confidence
- Zoom/pan controls
- Click node → show mini-card with entity preview
- Double-click → navigate to Detail View
- Filter controls: by label, by relation type, by depth
- Traversal mode: select start node, adjust depth slider

---

### 4. Entity Detail View
**Purpose:** Full view of a single memory with related entities.

**Components:**
- Header: entity ID, type badge
- Content: full text (markdown rendered)
- Metadata panel:
  - Labels (editable chips)
  - Confidence score (progress bar)
  - Source
  - Created / Last accessed dates
  - Access count
  - Decay weight indicator
- Related entities section:
  - Grouped by relation type (e.g., "uses →", "← used by")
  - Each shows: entity ID, content preview, relation weight
  - Click → navigate to that entity
- Actions:
  - Edit entity
  - Delete (with confirmation)
  - Check for contradictions
  - Find path to another entity

---

### 5. Add Memory Form
**Purpose:** Manually add new entities and relations.

**Components:**
- Entity form:
  - ID (auto-generated slug from content, or manual)
  - Content (textarea, required)
  - Type dropdown
  - Labels (tag input, autocomplete from existing)
  - Confidence slider
  - Source text field
- Relations form:
  - From entity (autocomplete search)
  - To entity (autocomplete search)
  - Relation type
  - Weight slider
- "Check contradictions before saving" toggle
- "Auto-tag with AI" toggle

---

### 6. File Upload / Ingestion
**Purpose:** Bulk import knowledge from documents.

**Components:**
- Drag-and-drop zone
- Supported formats: PDF, TXT, MD
- Processing status indicator
- Preview extracted entities before saving
- Auto-tagging option
- Assign default labels to batch

---

### 7. Maintenance / Reflect Panel
**Purpose:** Memory hygiene and optimization.

**Components:**
- "Run Reflect" button with options:
  - Apply decay (checkbox, days threshold input)
  - Find similar (checkbox, similarity threshold slider)
  - Auto-merge duplicates (checkbox)
- Results display:
  - Decayed memories count
  - Similar pairs found (list with merge/ignore actions)
  - Merged count
- Contradiction check results:
  - List of conflicting entity pairs
  - Confidence score for each contradiction
  - Actions: keep one, keep both, merge, delete

---

## Visual Design Guidelines

- **Color scheme:** Dark mode friendly (knowledge bases are often used during long sessions)
- **Typography:** Monospace for IDs, readable sans-serif for content
- **Graph colors:** Distinct colors per entity type or label category
- **Confidence indicators:** Green (high) → Yellow → Red (low)
- **Decay visualization:** Opacity or grayscale for stale memories
- **Responsive:** Works on desktop, tablet for graph exploration

---

## Sample Data for Development

```json
{
  "entities": [
    {
      "id": "user_pref_dark_mode",
      "content": "User prefers dark mode for all applications",
      "type": "preference",
      "labels": ["user", "ui", "preference"],
      "confidence": 0.95,
      "access_count": 12
    },
    {
      "id": "project_memcp",
      "content": "memcp is an MCP server providing persistent memory for AI agents using SurrealDB",
      "type": "project",
      "labels": ["project", "python", "mcp", "ai"],
      "confidence": 1.0,
      "access_count": 45
    },
    {
      "id": "tech_surrealdb",
      "content": "SurrealDB is a multi-model database supporting graphs, documents, and vectors",
      "type": "technology",
      "labels": ["database", "technology"],
      "confidence": 0.9,
      "access_count": 8
    }
  ],
  "relations": [
    {"from": "project_memcp", "to": "tech_surrealdb", "type": "uses", "weight": 1.0},
    {"from": "user_pref_dark_mode", "to": "project_memcp", "type": "applies_to", "weight": 0.8}
  ]
}
```

---

## Tech Stack Suggestion

- **Framework:** React/Next.js or Vue
- **Graph library:** D3.js, Cytoscape.js, or React Flow
- **UI components:** shadcn/ui, Radix, or similar
- **State:** React Query for API caching
- **API:** REST or GraphQL wrapper around MCP tools
