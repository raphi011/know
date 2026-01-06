
export enum EntityType {
  Concept = 'concept',
  Fact = 'fact',
  Preference = 'preference',
  Decision = 'decision',
  Requirement = 'requirement',
  Goal = 'goal',
  Problem = 'problem',
  Solution = 'solution',
  Question = 'question',
  Assumption = 'assumption',
  Observation = 'observation',
  Reference = 'reference',
  Person = 'person',
  Project = 'project'
}

export interface MemoryEntity {
  id: string;
  type: EntityType;
  content: string;
  confidence: number;
  lastAccessed: string;
  accessCount: number;
  labels: string[];
  context?: string;
  importance: number;
  user_importance?: number;
}

export interface Episode {
  id: string;
  content: string;
  summary?: string;
  timestamp: string;
  metadata?: Record<string, any>;
  context?: string;
  created: string;
  accessed: string;
  access_count: number;
}

export interface Procedure {
  id: string;
  name: string;
  description?: string;
  steps: { content: string; order?: number; metadata?: any }[];
  labels?: string[];
  context?: string;
  created: string;
  accessed: string;
  access_count: number;
}

export interface Relation {
  id: string;
  fromId: string;
  toId: string;
  type: 'related_to' | 'contradicts' | 'supports' | 'part_of' | 'is_a';
  weight: number;
}
