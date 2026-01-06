
import React, { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { backend } from '../backend';
import { Episode } from '../types';

const EpisodeDetail = () => {
  const { id } = useParams();
  const [episode, setEpisode] = useState<Episode | null>(null);

  useEffect(() => {
    if (id) backend.getEpisode(id).then(setEpisode);
  }, [id]);

  if (!episode) return <div className="flex-1 flex items-center justify-center bg-background-dark"><div className="size-8 border-2 border-primary border-t-transparent rounded-full animate-spin"></div></div>;

  return (
    <div className="flex flex-col h-full animate-page-in overflow-hidden">
      <header className="flex items-center justify-between px-6 py-4 border-b border-[#233f48] bg-background-dark/80 backdrop-blur-md sticky top-0 z-30">
        <div className="flex items-center gap-4">
          <Link to="/episodes" className="size-10 rounded-xl bg-surface-dark border border-[#233f48] flex items-center justify-center text-text-secondary hover:text-white transition-all">
            <span className="material-symbols-outlined">arrow_back</span>
          </Link>
          <div className="flex flex-col">
            <h2 className="text-white text-xl font-black leading-none uppercase tracking-tight">Episode: {episode.id}</h2>
            <p className="text-text-secondary text-xs mt-1 font-medium">{new Date(episode.timestamp).toLocaleString()}</p>
          </div>
        </div>
        <div className="flex items-center gap-3">
          <span className="bg-primary/10 text-primary border border-primary/20 px-3 py-1 rounded text-[10px] font-black uppercase tracking-widest">{episode.context}</span>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto p-8 md:p-12">
        <div className="max-w-4xl mx-auto grid grid-cols-1 lg:grid-cols-12 gap-10">
          <div className="lg:col-span-8 space-y-8">
            <section className="bg-surface-dark rounded-2xl border border-[#233f48] overflow-hidden shadow-2xl">
              <div className="px-6 py-4 border-b border-[#233f48] bg-black/10 flex justify-between items-center">
                 <h3 className="text-white font-black text-xs uppercase tracking-[0.2em]">Full Transcript</h3>
                 <span className="text-text-secondary text-[10px] font-mono">MD Format</span>
              </div>
              <div className="p-8 space-y-6">
                 {episode.content.split('\n').map((line, i) => (
                   <div key={i} className={`p-4 rounded-xl border ${line.startsWith('User:') ? 'bg-primary/5 border-primary/10' : 'bg-surface-dark-highlight/20 border-white/5'}`}>
                      <p className="text-sm leading-relaxed text-slate-200">
                        <span className="font-black text-primary mr-2 uppercase tracking-tighter">{line.split(':')[0]}:</span>
                        {line.split(':').slice(1).join(':')}
                      </p>
                   </div>
                 ))}
              </div>
            </section>
          </div>

          <aside className="lg:col-span-4 space-y-6">
            <div className="bg-surface-dark rounded-2xl border border-[#233f48] p-6 space-y-6 shadow-xl">
               <h4 className="text-white text-[10px] font-black uppercase tracking-[0.2em] border-b border-[#233f48] pb-4">Metadata Analysis</h4>
               <div className="space-y-4">
                  <div className="flex justify-between items-center text-xs">
                    <span className="text-text-secondary font-medium">Session ID</span>
                    <span className="text-white font-mono">{episode.id}</span>
                  </div>
                  <div className="flex justify-between items-center text-xs">
                    <span className="text-text-secondary font-medium">Namespace</span>
                    <span className="text-primary font-bold">{episode.context}</span>
                  </div>
                  <div className="flex justify-between items-center text-xs">
                    <span className="text-text-secondary font-medium">Total Tokens</span>
                    <span className="text-white font-bold">{episode.metadata?.tokens || 0}</span>
                  </div>
                  <div className="flex justify-between items-center text-xs">
                    <span className="text-text-secondary font-medium">Access Count</span>
                    <span className="text-white font-bold">{episode.access_count}</span>
                  </div>
               </div>
               <hr className="border-[#233f48]" />
               <div className="space-y-3">
                  <p className="text-[10px] font-black text-text-secondary uppercase tracking-widest">Linked Agents</p>
                  <div className="flex flex-wrap gap-2">
                    <span className="px-2 py-1 rounded-md bg-white/5 border border-white/10 text-[10px] text-slate-300 font-bold">{episode.metadata?.agent}</span>
                  </div>
               </div>
            </div>

            <div className="p-6 rounded-2xl bg-primary/5 border border-dashed border-primary/20 flex flex-col gap-3">
               <div className="flex items-center gap-2 text-primary">
                 <span className="material-symbols-outlined text-xl">insights</span>
                 <span className="text-[10px] font-black uppercase tracking-widest">Memory Impact</span>
               </div>
               <p className="text-[11px] text-text-secondary leading-relaxed">This session contributed <span className="text-white font-bold">14 new entities</span> and resolved <span className="text-white font-bold">2 conflicts</span> in the global reasoning pool.</p>
            </div>
          </aside>
        </div>
      </div>
    </div>
  );
};

export default EpisodeDetail;
