'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { graphql, QUERIES } from '@/lib/graphql';
import { useApp } from '@/components/AppProvider';
import { Procedure } from '@/lib/types';

export default function Procedures() {
  const { currentContext, showToast } = useApp();
  const [procedures, setProcedures] = useState<Procedure[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    const context = currentContext === 'all' ? null : currentContext;
    graphql<{ procedures: Procedure[] }>(QUERIES.procedures, { context, limit: 50 })
      .then(res => {
        setProcedures(res.procedures);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [currentContext]);

  return (
    <div className="flex flex-col h-full animate-page-in overflow-hidden">
      <header className="flex items-center justify-between px-6 py-4 border-b border-[#233f48] bg-background-dark/80 backdrop-blur-md sticky top-0 z-30">
        <div className="flex flex-col">
          <h2 className="text-white text-xl font-black leading-none uppercase tracking-tight">Procedural Store</h2>
          <p className="text-text-secondary text-xs mt-1 font-medium">Standard Protocols & Workflows</p>
        </div>
        <Link href="/procedure/new" className="bg-primary hover:bg-primary-dark text-[#101d22] font-black px-6 py-2 rounded-xl text-xs uppercase tracking-widest flex items-center gap-2 transition-all shadow-xl shadow-primary/20">
          <span className="material-symbols-outlined text-lg">add_circle</span> New Procedure
        </Link>
      </header>

      <div className="flex-1 overflow-y-auto p-6 md:p-10">
        <div className="max-w-6xl mx-auto">
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
            {loading ? (
              <div className="col-span-full flex justify-center py-20"><div className="size-8 border-2 border-primary border-t-transparent rounded-full animate-spin"></div></div>
            ) : procedures.map((proc) => (
              <div key={proc.id} className="bg-surface-dark border border-[#233f48] rounded-2xl p-6 flex flex-col gap-6 shadow-xl hover:border-primary/30 transition-all group">
                <div className="flex justify-between items-start">
                  <div className="size-12 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary group-hover:scale-110 transition-transform">
                    <span className="material-symbols-outlined text-2xl font-bold">terminal</span>
                  </div>
                  <div className="flex gap-2">
                    <Link href={`/procedure/${encodeURIComponent(proc.id)}`} className="size-8 rounded-lg bg-[#233f48] border border-white/5 flex items-center justify-center text-text-secondary hover:text-white">
                      <span className="material-symbols-outlined text-sm">edit</span>
                    </Link>
                    <span className="text-[10px] font-mono text-text-secondary opacity-40">{proc.id}</span>
                  </div>
                </div>
                <div>
                  <h3 className="text-white text-lg font-black tracking-tight mb-2 group-hover:text-primary transition-colors">{proc.name}</h3>
                  <p className="text-text-secondary text-xs leading-relaxed">{proc.description}</p>
                </div>
                <div className="space-y-3">
                  <div className="flex justify-between items-center text-[9px] font-black text-text-secondary uppercase tracking-widest border-b border-white/5 pb-2">
                    <span>Protocol Steps</span>
                    <span className="text-primary">{proc.steps.length} Nodes</span>
                  </div>
                  <div className="space-y-1.5">
                    {proc.steps.slice(0, 3).map((step, idx) => (
                      <div key={idx} className="flex gap-3 items-center text-[11px] text-slate-300 font-medium">
                        <span className="size-4 rounded-md bg-[#101d22] border border-[#233f48] flex items-center justify-center text-[9px] font-black text-primary">{idx + 1}</span>
                        <span className="truncate">{step.content}</span>
                      </div>
                    ))}
                    {proc.steps.length > 3 && <p className="text-[9px] text-text-secondary italic ml-7">+{proc.steps.length - 3} more steps...</p>}
                  </div>
                </div>
                <div className="mt-auto pt-6 flex justify-between items-center border-t border-white/5">
                  <div className="flex gap-1.5">
                    <span className="px-2 py-0.5 rounded-md bg-[#101d22] border border-white/5 text-[9px] font-black text-text-secondary uppercase">{proc.context}</span>
                  </div>
                  <button onClick={() => showToast(`Executing protocol: ${proc.name}`)} className="text-[10px] font-black text-primary hover:underline uppercase tracking-widest">Execute Protocol</button>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
