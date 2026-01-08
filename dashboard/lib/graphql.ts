const GRAPHQL_ENDPOINT = '/api/graphql';

export async function graphql<T>(
  query: string,
  variables?: Record<string, unknown>
): Promise<T> {
  const res = await fetch(GRAPHQL_ENDPOINT, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query, variables }),
    cache: 'no-store',
  });

  const json = await res.json();

  if (json.errors) {
    console.error('GraphQL errors:', json.errors);
    throw new Error(json.errors[0].message);
  }

  return json.data as T;
}

// GraphQL Queries
export const QUERIES = {
  contexts: `
    query {
      contexts
    }
  `,

  overview: `
    query Overview($context: String) {
      overview(context: $context) {
        stats { title value icon trend }
        growthData { name val }
        distribution { label val }
      }
    }
  `,

  recentMemories: `
    query RecentMemories($limit: Int) {
      recentMemories(limit: $limit) {
        id type content time icon importance
      }
    }
  `,

  episodes: `
    query Episodes($context: String, $limit: Int) {
      episodes(context: $context, limit: $limit) {
        id content summary context timestamp accessCount
      }
    }
  `,

  episode: `
    query Episode($id: String!) {
      episode(id: $id) {
        id content summary context timestamp accessCount metadata
      }
    }
  `,

  procedures: `
    query Procedures($context: String, $limit: Int) {
      procedures(context: $context, limit: $limit) {
        id name description steps { content optional } labels context
      }
    }
  `,

  procedure: `
    query Procedure($id: String!) {
      procedure(id: $id) {
        id name description steps { content optional } labels context
      }
    }
  `,

  searchMemories: `
    query Search($query: String!, $type: String, $context: String, $limit: Int) {
      searchMemories(query: $query, type: $type, context: $context, limit: $limit) {
        id type content labels score time access importance
      }
    }
  `,

  entity: `
    query Entity($id: String!) {
      entity(id: $id) {
        id type content labels confidence importance context accessCount
        neighbors { id type content }
      }
    }
  `,

  maintenanceData: `
    query {
      maintenanceData {
        health
        stats { total conflicts stale }
        conflicts { id content type similarity }
      }
    }
  `,
};

export const MUTATIONS = {
  saveProcedure: `
    mutation SaveProcedure($procedure: ProcedureInput!) {
      saveProcedure(procedure: $procedure) {
        id name description steps { content optional } labels context
      }
    }
  `,
};
