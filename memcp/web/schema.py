"""GraphQL schema for memcp web UI using Strawberry."""

from datetime import datetime
from typing import Any

import strawberry


# =============================================================================
# GraphQL Types - Mapping from mock backend.ts
# =============================================================================


@strawberry.type
class StatCard:
    """Dashboard stat card."""
    title: str
    value: str
    trend: str | None = None
    trend_icon: str | None = None
    icon: str
    color: str


@strawberry.type
class VelocityPoint:
    """Data point for velocity chart."""
    name: str
    val: int


@strawberry.type
class DistributionItem:
    """Item in entity type distribution."""
    label: str
    val: int
    color: str


@strawberry.type
class Overview:
    """Dashboard overview data."""
    stats: list[StatCard]
    velocity_data: list[VelocityPoint]
    distribution: list[DistributionItem]


@strawberry.type
class RecentMemory:
    """Recent memory for dashboard feed."""
    id: str
    type: str
    content: str
    time: str
    icon: str
    importance: float


@strawberry.type
class Episode:
    """Episode (conversation session)."""
    id: str
    content: str
    summary: str | None
    timestamp: str
    created: str
    accessed: str
    access_count: int
    context: str | None
    metadata: strawberry.scalars.JSON | None = None


@strawberry.type
class ProcedureStep:
    """Step in a procedure."""
    content: str
    optional: bool = False


@strawberry.type
class Procedure:
    """Stored procedure/workflow."""
    id: str
    name: str
    description: str
    steps: list[ProcedureStep]
    labels: list[str]
    context: str | None
    created: str
    accessed: str
    access_count: int


@strawberry.type
class SearchResult:
    """Search result entity."""
    id: str
    content: str
    score: float
    labels: list[str]
    time: str
    access: str
    importance: float
    type: str


@strawberry.type
class Neighbor:
    """Neighboring entity in graph."""
    id: str
    type: str
    content: str
    score: float


@strawberry.type
class Entity:
    """Full entity details."""
    id: str
    content: str
    type: str
    confidence: float
    last_accessed: str
    access_count: int
    labels: list[str]
    importance: float
    user_importance: float | None
    context: str | None
    neighbors: list[Neighbor]


@strawberry.type
class ConflictEntity:
    """Entity in a conflict."""
    id: str
    content: str
    time: str


@strawberry.type
class Conflict:
    """Detected contradiction/conflict."""
    id: str
    title: str
    mem_a: ConflictEntity
    mem_b: ConflictEntity
    similarity: float


@strawberry.type
class MaintenanceStats:
    """Maintenance statistics."""
    total: str
    conflicts: int
    stale: int


@strawberry.type
class MaintenanceData:
    """Full maintenance dashboard data."""
    health: int
    stats: MaintenanceStats
    conflicts: list[Conflict]


# =============================================================================
# Input Types
# =============================================================================


@strawberry.input
class ProcedureStepInput:
    """Input for procedure step."""
    content: str
    optional: bool = False


@strawberry.input
class ProcedureInput:
    """Input for saving a procedure."""
    id: str | None = None
    name: str
    description: str
    steps: list[ProcedureStepInput]
    labels: list[str] = strawberry.field(default_factory=list)
    context: str | None = None


# =============================================================================
# Query Resolvers - Will be implemented with real db calls
# =============================================================================


def format_time_ago(dt: datetime | str | None) -> str:
    """Format datetime as relative time string."""
    if dt is None:
        return "Unknown"

    if isinstance(dt, str):
        try:
            dt = datetime.fromisoformat(dt.replace('Z', '+00:00'))
        except ValueError:
            return dt

    now = datetime.now(dt.tzinfo) if dt.tzinfo else datetime.now()
    diff = now - dt

    seconds = diff.total_seconds()
    if seconds < 60:
        return "Just now"
    elif seconds < 3600:
        mins = int(seconds / 60)
        return f"{mins}m ago"
    elif seconds < 86400:
        hours = int(seconds / 3600)
        return f"{hours}h ago"
    else:
        days = int(seconds / 86400)
        return f"{days}d ago"


def format_datetime(dt: datetime | str | None) -> str:
    """Format datetime to ISO string."""
    if dt is None:
        return ""
    if isinstance(dt, str):
        return dt
    return dt.isoformat()
