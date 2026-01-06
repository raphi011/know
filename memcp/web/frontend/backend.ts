
import { EntityType, Episode, Procedure } from './types';

const delay = (ms: number) => new Promise(resolve => setTimeout(resolve, ms));

// Mock persistence for procedures
let _procedures: Procedure[] = [
  { 
    id: 'proc-01', 
    name: 'Vector Database Maintenance', 
    description: 'Weekly sweep of stale vectors and re-indexing protocol.', 
    steps: [
      { content: 'Identify stale nodes via access_count < 2' }, 
      { content: 'Run semantic decay algorithm on orphaned clusters' }, 
      { content: 'Re-index the remaining vector space' }
    ], 
    labels: ['ops', 'database'], 
    context: 'memcp', 
    created: '2024-01-01T00:00:00Z', 
    accessed: '2024-05-20T00:00:00Z', 
    access_count: 24 
  },
  { 
    id: 'proc-02', 
    name: 'Project Initialization', 
    description: 'Setup logic for new workspace namespaces.', 
    steps: [
      { content: 'Create project-level namespace in SurrealDB' }, 
      { content: 'Set default importance thresholds for incoming telemetry' }
    ], 
    labels: ['setup'], 
    context: 'all', 
    created: '2024-02-15T00:00:00Z', 
    accessed: '2024-05-18T00:00:00Z', 
    access_count: 12 
  },
];

export const backend = {
  listContexts: async () => {
    await delay(200);
    return ['all', 'memcp', 'project-helios', 'client-alpha', 'internal-ops'];
  },

  getOverview: async (context: string = 'all') => {
    await delay(500);
    return {
      stats: [
        { title: "Total Entities", value: context === 'all' ? "14,205" : "1,402", trend: "12%", trendIcon: "trending_up", icon: "psychology", color: "text-primary" },
        { title: "Total Relations", value: context === 'all' ? "32,891" : "3,110", trend: "5.2%", trendIcon: "trending_up", icon: "share", color: "text-purple-400" },
        { title: "Episodes Logged", value: context === 'all' ? "452" : "28", trend: "3.1%", trendIcon: "trending_up", icon: "history", color: "text-orange-400" },
      ],
      velocityData: Array.from({ length: 9 }).map((_, i) => ({ name: `${i*3}:00`, val: Math.floor(Math.random() * 3000) + 1000 })),
      distribution: [
        { label: 'Fact', val: 35, color: 'bg-primary' },
        { label: 'Preference', val: 25, color: 'bg-purple-400' },
        { label: 'Requirement', val: 20, color: 'bg-teal-400' },
        { label: 'Decision', val: 20, color: 'bg-yellow-400' }
      ]
    };
  },

  getRecentMemories: async () => {
    await delay(300);
    return [
      { id: 'mem_9x21', type: EntityType.Preference, content: 'User prefers dark mode for all IDE interfaces.', time: 'Just now', icon: 'brightness_4', importance: 0.85 },
      { id: 'mem_4k82', type: EntityType.Observation, content: 'Frequent mentions of "Project Helios" in recent sessions.', time: '4m ago', icon: 'visibility', importance: 0.62 },
      { id: 'mem_1z09', type: EntityType.Goal, content: 'Establish a self-sustaining memory decay protocol by Q4.', time: '12m ago', icon: 'flag', importance: 0.94 },
    ];
  },

  getEpisodes: async (context: string = 'all'): Promise<Episode[]> => {
    await delay(400);
    return [
      { id: 'sess-001', content: 'User: "Optimize the memory retrieval loop."\nAI: "Loop optimized using vector cache."', summary: 'Memory retrieval optimization', timestamp: '2024-05-20T10:00:00Z', created: '2024-05-20T10:00:00Z', accessed: '2024-05-20T10:05:00Z', access_count: 5, context: 'memcp' },
      { id: 'sess-002', content: 'User: "What is the status of Helios?"\nAI: "Helios is at 75% completion."', summary: 'Helios status check', timestamp: '2024-05-21T14:30:00Z', created: '2024-05-21T14:30:00Z', accessed: '2024-05-21T14:35:00Z', access_count: 2, context: 'project-helios' },
    ];
  },

  getEpisode: async (id: string): Promise<Episode | null> => {
    await delay(300);
    return { 
      id, 
      content: 'User: "Analyze current trends in LLM memory structures."\nAI: "Current trends emphasize episodic and procedural partitioning, much like the memcp architecture we are building."\nUser: "Interesting. How do we measure retrieval latency?"\nAI: "Retrieval latency is typically measured by p99 search time across the vector cluster."',
      summary: 'Memory Architecture Discussion', 
      timestamp: '2024-05-22T09:12:00Z', 
      created: '2024-05-22T09:12:00Z', 
      accessed: '2024-05-22T09:15:00Z', 
      access_count: 14, 
      context: 'memcp',
      metadata: { agent: 'claude-3-opus', tokens: 420, task: 'architecture-review' }
    };
  },

  getProcedures: async (context: string = 'all'): Promise<Procedure[]> => {
    await delay(400);
    return _procedures.filter(p => context === 'all' || p.context === context || p.context === 'all');
  },

  getProcedure: async (id: string): Promise<Procedure | null> => {
    await delay(300);
    return _procedures.find(p => p.id === id) || null;
  },

  saveProcedure: async (proc: Procedure) => {
    await delay(500);
    const idx = _procedures.findIndex(p => p.id === proc.id);
    if (idx >= 0) _procedures[idx] = proc;
    else _procedures.push(proc);
    return proc;
  },

  searchMemories: async (query: string, type?: EntityType, context: string = 'all') => {
    await delay(700);
    return [
      { id: "mem_8f92a10c", content: "User expressed a strong preference for <span class='text-primary font-bold underline decoration-primary/30'>Python</span> over Java.", score: 98, labels: ['Preference'], time: "2 mins ago", access: "42", importance: 0.92, type: EntityType.Preference },
      { id: "mem_b291x55z", content: "Constraint: All database migrations must happen during low-traffic windows.", score: 84, labels: ['Requirement'], time: "1 day ago", access: "5", importance: 0.78, type: EntityType.Requirement }
    ];
  },

  getEntity: async (id: string) => {
    await delay(400);
    return {
      id: id || 'ent-8f7a',
      content: "The user expressly stated a preference for using Python for all backend microservices.",
      type: EntityType.Preference,
      confidence: 0.98,
      lastAccessed: "2.4s ago",
      accessCount: 14,
      labels: ['engineering', 'stack'],
      importance: 0.88,
      user_importance: 0.7,
      context: 'memcp',
      neighbors: [
        { id: 'MEM_1A4Z', type: 'Supports', content: 'System documentation confirms Python 3.12 capability.', score: 0.94 }
      ]
    };
  },

  getMaintenanceData: async () => {
    await delay(600);
    return {
      health: 86,
      stats: { total: "12.4k", conflicts: 3, stale: 142 },
      conflicts: [
        { id: 'conf_01', title: 'User Preference', memA: { id: 'mem_8291', content: "I prefer latte.", time: "2 days ago" }, memB: { id: 'mem_9102', content: "I drink black coffee.", time: "Today" }, similarity: 0.92 }
      ]
    };
  }
};
