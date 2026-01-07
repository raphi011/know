export enum EntityType {
  Observation = 'observation',
  Preference = 'preference',
  Fact = 'fact',
  Decision = 'decision',
  Concept = 'concept',
  Requirement = 'requirement',
  Person = 'person',
  Organization = 'organization',
  Location = 'location',
  Event = 'event',
  Tool = 'tool',
  Project = 'project',
  Code = 'code',
  Procedure = 'procedure',
}

export interface MemoryEntity {
  id: string;
  type: EntityType;
  content: string;
  labels: string[];
  confidence?: number;
  importance?: number;
  context?: string;
  accessCount?: number;
  created?: string;
  accessed?: string;
}

export interface Episode {
  id: string;
  content: string;
  summary?: string;
  context?: string;
  timestamp?: string;
  accessCount?: number;
  metadata?: Record<string, unknown>;
}

export interface ProcedureStep {
  content: string;
  optional?: boolean;
  order?: number;
}

export interface Procedure {
  id: string;
  name: string;
  description: string;
  steps: ProcedureStep[];
  labels?: string[];
  context?: string;
  accessCount?: number;
}

export interface Relation {
  from: string;
  to: string;
  type: string;
  weight?: number;
}

export interface StatCard {
  title: string;
  value: string;
  icon: string;
  trend?: string;
}

export interface SearchResult {
  id: string;
  type: EntityType;
  content: string;
  labels: string[];
  score: number;
  time: string;
  access: string;
  importance: number;
}
