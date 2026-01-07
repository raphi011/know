"""FastAPI + Strawberry GraphQL server for memcp web UI."""

from __future__ import annotations

import asyncio
import logging
import os
from contextlib import asynccontextmanager
from pathlib import Path
from typing import AsyncIterator, TYPE_CHECKING

import strawberry
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles
from strawberry.fastapi import GraphQLRouter

if TYPE_CHECKING:
    from surrealdb import AsyncSurreal

from memcp.db import (
    SURREALDB_URL,
    SURREALDB_NAMESPACE,
    SURREALDB_DATABASE,
    SURREALDB_USER,
    SURREALDB_PASS,
    SURREALDB_AUTH_LEVEL,
    SCHEMA_SQL,
    query_count_entities,
    query_count_relations,
    query_count_episodes,
    query_count_procedures,
    query_list_contexts,
    query_get_context_stats,
    query_list_types,
    query_hybrid_search,
    query_get_entity,
    query_get_episode,
    query_list_procedures,
    query_get_procedure,
    query_search_procedures,
    query_apply_decay,
    query_all_entities_with_embedding,
    query_similar_by_embedding,
)
from memcp.api.schema import (
    StatCard,
    VelocityPoint,
    DistributionItem,
    Overview,
    RecentMemory,
    Episode,
    Procedure,
    ProcedureStep,
    SearchResult,
    Entity,
    Neighbor,
    MaintenanceData,
    MaintenanceStats,
    Conflict,
    ConflictEntity,
    ProcedureInput,
    format_time_ago,
    format_datetime,
)

logger = logging.getLogger("memcp.webui")

# Global database connection (managed by lifespan)
_db: "AsyncSurreal | None" = None


async def get_db() -> "AsyncSurreal":
    """Get the database connection."""
    if _db is None:
        raise RuntimeError("Database not initialized")
    return _db


def _extract_count(result) -> int:
    """Extract count from SurrealDB query result."""
    try:
        if not result:
            return 0
        data = result[0] if result else {}
        if isinstance(data, dict):
            return data.get("count", 0)
        return 0
    except Exception:
        return 0


# =============================================================================
# GraphQL Query Resolvers
# =============================================================================


@strawberry.type
class Query:
    @strawberry.field
    async def contexts(self) -> list[str]:
        """List all available contexts."""
        db = await get_db()
        result = await query_list_contexts(db)
        if result and isinstance(result[0], dict):
            return result[0].get("contexts", [])
        return []

    @strawberry.field
    async def overview(self, context: str | None = None) -> Overview:
        """Get dashboard overview data."""
        db = await get_db()

        # Get counts
        entity_result = await query_count_entities(db, context)
        relation_result = await query_count_relations(db, context)
        episode_result = await query_count_episodes(db, context)

        entity_count = _extract_count(entity_result)
        relation_count = _extract_count(relation_result)
        episode_count = _extract_count(episode_result)

        stats = [
            StatCard(
                title="Total Entities",
                value=f"{entity_count:,}",
                trend="12%",
                trend_icon="trending_up",
                icon="psychology",
                color="text-primary",
            ),
            StatCard(
                title="Total Relations",
                value=f"{relation_count:,}",
                trend="5.2%",
                trend_icon="trending_up",
                icon="share",
                color="text-purple-400",
            ),
            StatCard(
                title="Episodes Logged",
                value=f"{episode_count:,}",
                trend="3.1%",
                trend_icon="trending_up",
                icon="history",
                color="text-orange-400",
            ),
        ]

        # Get type distribution
        type_result = await query_list_types(db, context)
        distribution = []
        colors = ["bg-primary", "bg-purple-400", "bg-teal-400", "bg-yellow-400", "bg-red-400"]
        for i, item in enumerate(type_result or []):
            if isinstance(item, dict):
                distribution.append(
                    DistributionItem(
                        label=item.get("type", "unknown").capitalize(),
                        val=item.get("count", 0),
                        color=colors[i % len(colors)],
                    )
                )

        # Mock velocity data for now (would need time-series query)
        import random
        velocity_data = [
            VelocityPoint(name=f"{i*3}:00", val=random.randint(100, 500))
            for i in range(9)
        ]

        return Overview(stats=stats, velocity_data=velocity_data, distribution=distribution)

    @strawberry.field
    async def recent_memories(self, limit: int = 5) -> list[RecentMemory]:
        """Get recent memories for dashboard feed."""
        db = await get_db()
        from memcp.db import run_query

        result = await run_query(
            db,
            """
            SELECT id, type, content, importance, accessed
            FROM entity
            ORDER BY accessed DESC
            LIMIT $limit
        """,
            {"limit": limit},
        )

        memories = []
        for item in result or []:
            if isinstance(item, dict):
                entity_id = str(item.get("id", "")).split(":")[-1]
                memories.append(
                    RecentMemory(
                        id=entity_id,
                        type=item.get("type", "concept"),
                        content=item.get("content", "")[:100],
                        time=format_time_ago(item.get("accessed")),
                        icon="memory",
                        importance=item.get("importance", 0.5),
                    )
                )
        return memories

    @strawberry.field
    async def episodes(self, context: str | None = None, limit: int = 50) -> list[Episode]:
        """List episodes."""
        db = await get_db()
        from memcp.db import run_query

        context_filter = "WHERE context = $ctx" if context and context != "all" else ""
        result = await run_query(
            db,
            f"""
            SELECT * FROM episode {context_filter}
            ORDER BY timestamp DESC
            LIMIT $limit
        """,
            {"ctx": context, "limit": limit},
        )

        episodes = []
        for item in result or []:
            if isinstance(item, dict):
                episode_id = str(item.get("id", "")).split(":")[-1]
                episodes.append(
                    Episode(
                        id=episode_id,
                        content=item.get("content", ""),
                        summary=item.get("summary"),
                        timestamp=format_datetime(item.get("timestamp")),
                        created=format_datetime(item.get("created")),
                        accessed=format_datetime(item.get("accessed")),
                        access_count=item.get("access_count", 0),
                        context=item.get("context"),
                        metadata=item.get("metadata"),
                    )
                )
        return episodes

    @strawberry.field
    async def episode(self, id: str) -> Episode | None:
        """Get episode by ID."""
        db = await get_db()
        result = await query_get_episode(db, id)
        if not result:
            return None

        item = result[0]
        if not isinstance(item, dict):
            return None

        return Episode(
            id=id,
            content=item.get("content", ""),
            summary=item.get("summary"),
            timestamp=format_datetime(item.get("timestamp")),
            created=format_datetime(item.get("created")),
            accessed=format_datetime(item.get("accessed")),
            access_count=item.get("access_count", 0),
            context=item.get("context"),
            metadata=item.get("metadata"),
        )

    @strawberry.field
    async def procedures(self, context: str | None = None, limit: int = 50) -> list[Procedure]:
        """List procedures."""
        db = await get_db()
        ctx = context if context and context != "all" else None
        result = await query_list_procedures(db, ctx, limit)

        procedures = []
        for item in result or []:
            if isinstance(item, dict):
                proc_id = str(item.get("id", "")).split(":")[-1]
                steps = [
                    ProcedureStep(
                        content=s.get("content", ""),
                        optional=s.get("optional", False),
                    )
                    for s in item.get("steps", [])
                    if isinstance(s, dict)
                ]
                procedures.append(
                    Procedure(
                        id=proc_id,
                        name=item.get("name", ""),
                        description=item.get("description", ""),
                        steps=steps,
                        labels=item.get("labels", []),
                        context=item.get("context"),
                        created=format_datetime(item.get("created")),
                        accessed=format_datetime(item.get("accessed")),
                        access_count=item.get("access_count", 0),
                    )
                )
        return procedures

    @strawberry.field
    async def procedure(self, id: str) -> Procedure | None:
        """Get procedure by ID."""
        db = await get_db()
        result = await query_get_procedure(db, id)
        if not result:
            return None

        item = result[0]
        if not isinstance(item, dict):
            return None

        steps = [
            ProcedureStep(
                content=s.get("content", ""),
                optional=s.get("optional", False),
            )
            for s in item.get("steps", [])
            if isinstance(s, dict)
        ]

        return Procedure(
            id=id,
            name=item.get("name", ""),
            description=item.get("description", ""),
            steps=steps,
            labels=item.get("labels", []),
            context=item.get("context"),
            created=format_datetime(item.get("created")),
            accessed=format_datetime(item.get("accessed")),
            access_count=item.get("access_count", 0),
        )

    @strawberry.field
    async def search_memories(
        self,
        query: str,
        type: str | None = None,
        context: str | None = None,
        limit: int = 20,
    ) -> list[SearchResult]:
        """Search memories with hybrid search."""
        db = await get_db()
        from memcp.utils import embed

        query_embedding = embed(query)
        ctx = context if context and context != "all" else None
        result = await query_hybrid_search(db, query, query_embedding, [], limit, ctx)

        results = []
        for item in result or []:
            if isinstance(item, dict):
                entity_id = str(item.get("id", "")).split(":")[-1]
                entity_type = item.get("type", "concept")

                # Filter by type if specified
                if type and entity_type.lower() != type.lower():
                    continue

                results.append(
                    SearchResult(
                        id=entity_id,
                        content=item.get("content", ""),
                        score=item.get("score", 0.0) if "score" in item else 0.8,
                        labels=item.get("labels", []),
                        time=format_time_ago(item.get("accessed")),
                        access=str(item.get("access_count", 0)),
                        importance=item.get("importance", 0.5),
                        type=entity_type,
                    )
                )
        return results

    @strawberry.field
    async def entity(self, id: str) -> Entity | None:
        """Get entity by ID."""
        db = await get_db()
        result = await query_get_entity(db, id)
        if not result:
            return None

        item = result[0]
        if not isinstance(item, dict):
            return None

        # Get neighbors (related entities)
        from memcp.db import run_query

        neighbor_result = await run_query(
            db,
            """
            SELECT out.id AS id, out.type AS type, out.content AS content, weight AS score
            FROM relates
            WHERE in = type::record("entity", $id)
            LIMIT 10
        """,
            {"id": id},
        )

        neighbors = []
        for n in neighbor_result or []:
            if isinstance(n, dict):
                n_id = str(n.get("id", "")).split(":")[-1]
                neighbors.append(
                    Neighbor(
                        id=n_id,
                        type=n.get("type", "concept"),
                        content=n.get("content", "")[:100],
                        score=n.get("score", 1.0),
                    )
                )

        return Entity(
            id=id,
            content=item.get("content", ""),
            type=item.get("type", "concept"),
            confidence=item.get("confidence", 1.0),
            last_accessed=format_time_ago(item.get("accessed")),
            access_count=item.get("access_count", 0),
            labels=item.get("labels", []),
            importance=item.get("importance", 0.5),
            user_importance=item.get("user_importance"),
            context=item.get("context"),
            neighbors=neighbors,
        )

    @strawberry.field
    async def maintenance_data(self) -> MaintenanceData:
        """Get maintenance dashboard data."""
        db = await get_db()
        from memcp.db import run_query

        # Get total entities
        entity_result = await query_count_entities(db)
        total = _extract_count(entity_result)

        # Count stale entities (not accessed in 30 days)
        stale_result = await run_query(
            db,
            """
            SELECT count() FROM entity
            WHERE accessed < time::now() - 30d
            GROUP ALL
        """,
        )
        stale = _extract_count(stale_result)

        # Calculate health score (simplified)
        health = 100 - min(50, int(stale / max(1, total) * 100))

        return MaintenanceData(
            health=health,
            stats=MaintenanceStats(
                total=f"{total:,}" if total < 10000 else f"{total/1000:.1f}k",
                conflicts=0,  # Would need contradiction detection
                stale=stale,
            ),
            conflicts=[],  # Would need contradiction detection
        )


@strawberry.type
class Mutation:
    @strawberry.mutation
    async def save_procedure(self, procedure: ProcedureInput) -> Procedure:
        """Save (create or update) a procedure."""
        db = await get_db()
        from memcp.db import query_create_procedure
        from memcp.utils import embed
        import uuid

        proc_id = procedure.id or str(uuid.uuid4())[:8]
        steps = [{"content": s.content, "optional": s.optional} for s in procedure.steps]
        embedding = embed(f"{procedure.name} {procedure.description}")

        await query_create_procedure(
            db,
            proc_id,
            procedure.name,
            procedure.description,
            steps,
            embedding,
            procedure.context,
            procedure.labels,
        )

        # Fetch and return the saved procedure
        from memcp.db import query_get_procedure

        result = await query_get_procedure(db, proc_id)
        item = result[0] if result else {}

        return Procedure(
            id=proc_id,
            name=procedure.name,
            description=procedure.description,
            steps=[ProcedureStep(content=s.content, optional=s.optional) for s in procedure.steps],
            labels=procedure.labels,
            context=procedure.context,
            created=format_datetime(item.get("created")) if item else "",
            accessed=format_datetime(item.get("accessed")) if item else "",
            access_count=item.get("access_count", 0) if item else 0,
        )


# =============================================================================
# FastAPI Application
# =============================================================================


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Manage database connection lifecycle."""
    global _db
    from surrealdb import AsyncSurreal

    logger.info(f"Connecting to SurrealDB at {SURREALDB_URL}...")
    _db = AsyncSurreal(SURREALDB_URL)

    try:
        await _db.connect()

        # Authenticate
        if SURREALDB_AUTH_LEVEL == "root":
            await _db.signin({"username": SURREALDB_USER, "password": SURREALDB_PASS})
        else:
            await _db.signin({
                "namespace": SURREALDB_NAMESPACE,
                "database": SURREALDB_DATABASE,
                "username": SURREALDB_USER,
                "password": SURREALDB_PASS,
            })

        await _db.use(SURREALDB_NAMESPACE, SURREALDB_DATABASE)

        # Initialize schema
        await _db.query(SCHEMA_SQL)

        logger.info("Database connected and schema initialized")
        yield

    finally:
        if _db:
            await _db.close()
            _db = None


def create_app() -> FastAPI:
    """Create the FastAPI application."""
    app = FastAPI(
        title="memcp Web UI",
        description="GraphQL API for memcp memory browser",
        version="0.1.0",
        lifespan=lifespan,
    )

    # CORS for development (React dev server on different port)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["http://localhost:5173", "http://localhost:3000", "http://127.0.0.1:5173"],
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    # GraphQL endpoint
    schema = strawberry.Schema(query=Query, mutation=Mutation)
    graphql_app = GraphQLRouter(schema)
    app.include_router(graphql_app, prefix="/graphql")

    # Static files for React SPA (if build exists)
    static_dir = Path(__file__).parent / "static"
    if static_dir.exists():
        # Serve static files, with fallback to index.html for SPA routing
        app.mount("/", StaticFiles(directory=static_dir, html=True), name="static")

    return app


def run(host: str = "0.0.0.0", port: int = 8080, reload: bool = False):
    """Run the web UI server."""
    import uvicorn

    uvicorn.run(
        "memcp.webui.main:create_app",
        host=host,
        port=port,
        reload=reload,
        factory=True,
    )


if __name__ == "__main__":
    run(reload=True)
