'use client';

import { useState, useEffect } from 'react';
import Link from 'next/link';
import { graphql, QUERIES } from '@/lib/graphql';
import { useApp } from '@/components/AppProvider';
import { EntityType, SearchResult } from '@/lib/types';

function SearchResultCard({ id, content, score, labels, type }: SearchResult) {
  return (
    <Link href={`/entity/${encodeURIComponent(id)}`} className="group bg-surface-dark rounded-xl p-6 border border-transparent hover:border-primary/40 transition-all shadow-xl block relative">
      <div className="flex items-start justify-between mb-4">
        <div className="flex items-center gap-3">
          <div className="size-9 rounded-lg bg-primary/10 flex items-center justify-center text-primary border border-primary/20">
            <span className="material-symbols-outlined text-[20px]">psychology</span>
          </div>
          <div>
            <h3 className="text-sm font-mono font-bold text-text-secondary leading-none">{id}</h3>
            <span className="text-[9px] font-black text-primary/60 uppercase tracking-widest mt-1 block">{type}</span>
          </div>
        </div>
        <div className="flex flex-col items-end gap-2">
          <span className="inline-flex items-center px-2.5 py-1 rounded-md text-[10px] font-black uppercase tracking-widest bg-primary/10 text-primary border border-primary/20 shadow-inner">
            {Math.round(score * 100)}% Similarity
          </span>
        </div>
      </div>
      <div className="mb-6">
        <p className="text-base text-slate-300 leading-relaxed font-medium">{content}</p>
      </div>
      <div className="flex flex-wrap items-center gap-y-3 gap-x-6 border-t border-white/5 pt-4">
        <div className="flex items-center gap-2">
          <span className="material-symbols-outlined text-[16px] text-text-secondary">label</span>
          {labels.map((l: string) => (
            <span key={l} className="text-[10px] font-black uppercase tracking-widest text-slate-300 bg-black/30 px-2 py-1 rounded-md border border-white/5">{l}</span>
          ))}
        </div>
      </div>
    </Link>
  );
}

export default function Search() {
  const { currentContext } = useApp();
  const [query, setQuery] = useState('');
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [filterType, setFilterType] = useState<string>('all');

  const handleSearch = async () => {
    setLoading(true);
    try {
      const context = currentContext === 'all' ? null : currentContext;
      const type = filterType === 'all' ? null : filterType;
      const res = await graphql<{ searchMemories: SearchResult[] }>(QUERIES.searchMemories, { query, type, context, limit: 20 });
      setResults(res.searchMemories);
    } catch (error) {
      console.error('Search failed:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    handleSearch();
  }, [currentContext, filterType]);

  return (
    <div className="flex flex-col h-full bg-background-dark items-center animate-page-in overflow-y-auto">
      <header className="w-full flex items-center justify-between px-6 py-4 border-b border-[#233f48] bg-background-dark/80 backdrop-blur-md sticky top-0 z-30">
        <h2 className="text-white text-xl font-black uppercase tracking-tight">Knowledge Explorer</h2>
        <div className="flex items-center gap-4">
          <span className="text-[10px] font-black text-text-secondary uppercase tracking-widest bg-[#1a2c35] px-3 py-1.5 rounded-lg border border-white/5">Namespace: {currentContext.toUpperCase()}</span>
        </div>
      </header>

      <div className="w-full max-w-[1100px] flex flex-col gap-8 py-10 px-6">
        <div className="bg-surface-dark rounded-2xl border border-[#233f48] p-4 flex flex-col gap-4 shadow-2xl">
          <div className="relative w-full">
            <label className="flex items-center w-full h-16 bg-[#1a323a] rounded-xl px-5 border border-[#233f48] focus-within:border-primary/50 focus-within:ring-4 focus-within:ring-primary/5 transition-all">
              <span className="material-symbols-outlined text-text-secondary text-[28px]">search</span>
              <input
                className="w-full bg-transparent border-none focus:ring-0 text-white placeholder:text-text-secondary text-lg px-4 font-medium outline-none"
                placeholder="Query persistent knowledge..."
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
              />
              <button className="text-text-secondary hover:text-white transition-colors p-2" onClick={() => setQuery('')}>
                <span className="material-symbols-outlined text-[20px]">close</span>
              </button>
            </label>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-6 p-4 bg-black/10 rounded-xl border border-white/5">
            <div className="flex flex-col gap-2">
              <label className="text-[10px] font-black uppercase tracking-widest text-text-secondary">Filter by Ontology</label>
              <select
                className="bg-[#101d22] border border-[#233f48] rounded-lg text-xs font-bold text-white px-3 py-2 outline-none cursor-pointer"
                value={filterType}
                onChange={(e) => setFilterType(e.target.value)}
              >
                <option value="all">ALL ENTITIES</option>
                {Object.values(EntityType).map(t => <option key={t} value={t}>{t.toUpperCase()}</option>)}
              </select>
            </div>
            <div className="flex flex-col gap-2">
              <label className="text-[10px] font-black uppercase tracking-widest text-text-secondary">Sort Heuristic</label>
              <select className="bg-[#101d22] border border-[#233f48] rounded-lg text-xs font-bold text-white px-3 py-2 outline-none cursor-pointer">
                <option>IMPORTANCE (DESC)</option>
                <option>RECENCY</option>
                <option>SIMILARITY</option>
              </select>
            </div>
            <div className="flex items-end">
              <button onClick={handleSearch} className="w-full h-10 bg-primary hover:bg-primary-dark text-[#101d22] rounded-lg text-xs font-black uppercase tracking-widest transition-all shadow-lg shadow-primary/20">Execute Query</button>
            </div>
          </div>
        </div>

        <div className={`flex flex-col gap-6 transition-opacity duration-300 ${loading ? 'opacity-40' : 'opacity-100'}`}>
          {results.map((res) => (
            <SearchResultCard key={res.id} {...res} />
          ))}
          {results.length === 0 && !loading && (
            <div className="py-20 text-center flex flex-col items-center gap-4">
              <span className="material-symbols-outlined text-text-secondary text-5xl">inventory_2</span>
              <p className="text-text-secondary font-bold uppercase tracking-widest text-sm">No entities match the current query and filters.</p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
