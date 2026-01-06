"""Pydantic response models for memcp."""

from pydantic import BaseModel, Field


class EntityResult(BaseModel):
    """A memory entity returned from search or retrieval."""
    id: str
    type: str | None = None
    labels: list[str] = Field(default_factory=list)
    content: str
    confidence: float | None = None
    source: str | None = None
    decay_weight: float | None = None
    context: str | None = None  # Project namespace
    importance: float | None = None  # Heuristic-based salience score


class SearchResult(BaseModel):
    """Result from a memory search."""
    entities: list[EntityResult] = Field(default_factory=list)
    count: int = 0
    summary: str | None = None


class SimilarPair(BaseModel):
    """A pair of similar entities found during reflection."""
    entity1: EntityResult
    entity2: EntityResult
    similarity: float


class Contradiction(BaseModel):
    """A contradiction detected between two entities."""
    entity1: EntityResult
    entity2: EntityResult
    confidence: float


class ReflectResult(BaseModel):
    """Result from the reflect maintenance operation."""
    decayed: int = 0
    similar_pairs: list[SimilarPair] = Field(default_factory=list)
    merged: int = 0
    importance_recalculated: int = 0  # Count of entities with recalculated importance


class RememberResult(BaseModel):
    """Result from storing memories."""
    entities_stored: int = 0
    relations_stored: int = 0
    contradictions: list[Contradiction] = Field(default_factory=list)


class MemoryStats(BaseModel):
    """Statistics about the memory store."""
    total_entities: int = 0
    total_relations: int = 0
    labels: list[str] = Field(default_factory=list)
    label_counts: dict[str, int] = Field(default_factory=dict)


# =============================================================================
# Episode Models (Episodic Memory)
# =============================================================================

class EpisodeResult(BaseModel):
    """An episode returned from search or retrieval."""
    id: str
    content: str
    timestamp: str
    summary: str | None = None
    metadata: dict = Field(default_factory=dict)
    context: str | None = None
    linked_entities: int = 0  # Count of entities extracted from this episode
    entities: list[EntityResult] | None = None  # Optional: full entity data


class EpisodeSearchResult(BaseModel):
    """Result from episode search."""
    episodes: list[EpisodeResult] = Field(default_factory=list)
    count: int = 0


# =============================================================================
# Context Models (Project Namespacing)
# =============================================================================

class ContextStats(BaseModel):
    """Statistics for a specific context/namespace."""
    context: str
    entities: int = 0
    episodes: int = 0
    relations: int = 0


class ContextListResult(BaseModel):
    """List of all available contexts."""
    contexts: list[str] = Field(default_factory=list)
    count: int = 0
