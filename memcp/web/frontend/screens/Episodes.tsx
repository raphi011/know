
import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { backend } from '../backend';
import { useApp } from '../App';
import { Episode } from '../types';

const Episodes = () => {
  const { currentContext } = useApp();
  const [episodes, setEpisodes] = useState<Episode[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    backend.getEpisodes(currentContext).then(res => {
      setEpisodes(res);
      setLoading(false);
    });
  }, [currentContext]);

  return (
    <div className="flex flex-col h-full animate-page-in overflow-hidden">
      <header className="flex items-center justify-between px-6 py-4 border-b border-[#233f48] bg-background-dark/80 backdrop-blur-md sticky top-0 z-30">
        <div className="flex flex-col">
          <h2 className="text-white text-xl font-black leading-none uppercase tracking-tight">Episodic Logs</h2>
          <p className="text-text-secondary text-xs mt-1 font-medium">System Interaction History - Context: {currentContext.toUpperCase()}</p>
        </div>
        <div className="flex items-center gap-4">
           <div className="flex bg-surface-dark border border-[#233f48] rounded-lg p-1 shadow-inner">
             <button className="px-4 py-1.5 text-[10px] font-black bg-primary text-[#101d22] rounded-md uppercase">List View</button>
             <button className="px-4 py-1.5 text-[10px] font-black text-text-secondary hover:text-white uppercase transition-colors">Calendar</button>
           </div>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto p-6 md:p-10">
        <div className="max-w-5xl mx-auto space-y-8">
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4 p-4 bg-surface-dark border border-[#233f48] rounded-2xl shadow-inner">
             <div className="flex flex-col gap-1.5">
               <label className="text-[9px] font-black text-text-secondary uppercase tracking-widest">Search Episodes</label>
               <input className="bg-[#101d22] border border-[#233f48] rounded-lg px-3 py-2 text-xs text-white" placeholder="Keywords..." />
             </div>
             <div className="flex flex-col gap-1.5">
               <label className="text-[9px] font-black text-text-secondary uppercase tracking-widest">Time Range</label>
               <select className="bg-[#101d22] border border-[#233f48] rounded-lg px-3 py-2 text-xs text-white uppercase font-bold">
                 <option>Last 24 Hours</option>
                 <option>Last 7 Days</option>
                 <option>Historical</option>
               </select>
             </div>
             <div className="md:col-span-2 flex items-end">
               <button className="w-full h-9 bg-primary/10 border border-primary/20 text-primary hover:bg-primary hover:text-[#101d22] rounded-lg text-xs font-black uppercase tracking-widest transition-all">Filter logs</button>
             </div>
          </div>

          <div className="space-y-6">
            {loading ? (
              <div className="flex justify-center p-20"><div className="size-8 border-2 border-primary border-t-transparent rounded-full animate-spin"></div></div>
            ) : episodes.map((episode) => (
              <Link to={`/episode/${episode.id}`} key={episode.id} className="bg-surface-dark rounded-2xl border border-[#233f48] overflow-hidden group hover:border-primary/40 transition-all shadow-xl block">
                 <div className="px-6 py-4 border-b border-[#233f48] bg-black/10 flex justify-between items-center">
                    <div className="flex items-center gap-4">
                       <span className="material-symbols-outlined text-primary text-xl">forum</span>
                       <h3 className="text-white font-bold text-sm">{episode.summary || 'Interaction Episode'}</h3>
                       <span className="text-[10px] font-mono text-text-secondary bg-[#101d22] px-2 py-0.5 rounded border border-white/5 uppercase">{episode.id}</span>
                    </div>
                    <div className="flex items-center gap-4">
                       <span className="text-[10px] font-black text-text-secondary uppercase tracking-widest">{new Date(episode.timestamp).toLocaleString()}</span>
                       <div className="size-8 rounded-lg bg-[#233f48] flex items-center justify-center text-text-secondary group-hover:bg-primary group-hover:text-[#101d22] transition-colors cursor-pointer">
                          <span className="material-symbols-outlined text-lg">open_in_new</span>
                       </div>
                    </div>
                 </div>
                 <div className="p-6">
                    <div className="bg-[#101d22] rounded-xl p-5 border border-white/5 font-mono text-xs text-slate-400 leading-relaxed whitespace-pre-wrap max-h-40 overflow-y-auto">
                       {episode.content}
                    </div>
                    <div className="mt-4 flex flex-wrap gap-2 items-center">
                       <span className="text-[10px] font-black text-text-secondary uppercase tracking-widest mr-2">Contextual Tags:</span>
                       <span className="px-2 py-0.5 rounded-md bg-purple-500/10 text-purple-400 border border-purple-500/20 text-[9px] font-black uppercase">{episode.context}</span>
                       <span className="px-2 py-0.5 rounded-md bg-white/5 text-text-secondary border border-white/10 text-[9px] font-black uppercase">Session</span>
                       <div className="ml-auto text-[10px] font-bold text-text-secondary flex items-center gap-2">
                          <span className="material-symbols-outlined text-xs">visibility</span>
                          {episode.access_count} Accesses
                       </div>
                    </div>
                 </div>
              </Link>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};

export default Episodes;
