
import React, { useEffect, useState } from 'react';
import { useApp } from '../App';
import { backend } from '../backend';

const Maintenance = () => {
  const { showToast } = useApp();
  const [data, setData] = useState<any>(null);
  const [config, setConfig] = useState({
    applyDecay: true,
    findSimilar: true,
    recalcImportance: true
  });

  useEffect(() => {
    const fetchData = async () => {
      const result = await backend.getMaintenanceData();
      setData(result);
    };
    fetchData();
  }, []);

  const handleRunReflect = () => {
    showToast('Reflection engine started. Recalculating importance scores...', 'warning');
  };

  const handleResolve = (action: string) => {
    showToast(`Conflict resolved: ${action} applied`, 'success');
  };

  if (!data) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-background-dark text-text-secondary">
        <div className="size-8 border-2 border-primary border-t-transparent rounded-full animate-spin"></div>
      </div>
    );
  }

  return (
    <div className="p-6 md:p-10 max-w-7xl mx-auto w-full flex flex-col gap-10 h-full overflow-y-auto animate-page-in">
      <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-6">
        <div className="flex flex-col gap-2">
          <h1 className="text-4xl font-black text-white tracking-tight leading-none uppercase">System Integrity</h1>
          <p className="text-text-secondary text-lg">Knowledge consolidation, memory decay, and importance score recalibration.</p>
        </div>
        <div className="flex items-center gap-3 bg-green-500/10 px-4 py-2 rounded-full border border-green-500/20 shadow-inner">
          <span className="material-symbols-outlined text-green-400">check_circle</span>
          <span className="text-green-300 font-bold text-sm uppercase tracking-widest">Storage: Optimal</span>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-12 gap-10">
        <div className="lg:col-span-4 flex flex-col gap-6">
          <div className="bg-surface-dark border border-[#233f48] rounded-2xl overflow-hidden shadow-2xl">
            <div className="p-5 border-b border-[#233f48] bg-black/10">
              <h3 className="text-white text-lg font-bold flex items-center gap-3">
                <span className="material-symbols-outlined text-primary">auto_fix_high</span> Reflect Engine
              </h3>
            </div>
            <div className="p-6 space-y-4">
              <label className="flex items-start gap-4 p-4 rounded-xl hover:bg-white/5 cursor-pointer transition-colors border border-transparent hover:border-white/5 group">
                <input 
                  type="checkbox" 
                  checked={config.applyDecay} 
                  onChange={(e) => setConfig({...config, applyDecay: e.target.checked})}
                  className="mt-1 rounded border-[#325a67] bg-transparent text-primary focus:ring-0 size-5 cursor-pointer" 
                />
                <div>
                  <p className="text-white font-bold leading-none uppercase text-xs tracking-wider">Apply Memory Decay</p>
                  <p className="text-text-secondary text-[11px] mt-2 leading-relaxed">Systematically prune low-recall entities based on access patterns.</p>
                </div>
              </label>
              <label className="flex items-start gap-4 p-4 rounded-xl hover:bg-white/5 cursor-pointer transition-colors border border-transparent hover:border-white/5 group">
                <input 
                  type="checkbox" 
                  checked={config.recalcImportance}
                  onChange={(e) => setConfig({...config, recalcImportance: e.target.checked})}
                  className="mt-1 rounded border-[#325a67] bg-transparent text-primary focus:ring-0 size-5 cursor-pointer" 
                />
                <div>
                  <p className="text-white font-bold leading-none uppercase text-xs tracking-wider">Recalculate Importance</p>
                  <p className="text-text-secondary text-[11px] mt-2 leading-relaxed">Update heuristic scores based on connectivity and user interaction.</p>
                </div>
              </label>
              <label className="flex items-start gap-4 p-4 rounded-xl hover:bg-white/5 cursor-pointer transition-colors border border-transparent hover:border-white/5 group">
                <input 
                  type="checkbox" 
                  checked={config.findSimilar}
                  onChange={(e) => setConfig({...config, findSimilar: e.target.checked})}
                  className="mt-1 rounded border-[#325a67] bg-transparent text-primary focus:ring-0 size-5 cursor-pointer" 
                />
                <div>
                  <p className="text-white font-bold leading-none uppercase text-xs tracking-wider">Collision Detection</p>
                  <p className="text-text-secondary text-[11px] mt-2 leading-relaxed">Identify potentially redundant or contradicting memories for merging.</p>
                </div>
              </label>
            </div>
            <div className="p-6 border-t border-[#233f48] bg-black/10 flex flex-col gap-3">
               <button 
                onClick={handleRunReflect}
                className="w-full bg-primary hover:bg-primary-dark text-[#101d22] h-12 rounded-xl font-black text-base shadow-xl shadow-primary/20 flex items-center justify-center gap-3 transition-all uppercase tracking-widest"
               >
                  <span className="material-symbols-outlined text-xl">psychology</span> Trigger Reflect
               </button>
               <p className="text-center text-[10px] font-bold text-text-secondary uppercase tracking-[0.2em] opacity-40">Next Auto-Run: 4h 12m</p>
            </div>
          </div>

          <div className="bg-surface-dark border border-[#233f48] rounded-2xl p-8 shadow-2xl space-y-6">
             <h4 className="text-white text-[10px] font-black uppercase tracking-[0.2em] flex items-center gap-3 opacity-60">
               <span className="material-symbols-outlined text-primary text-lg">health_and_safety</span> Semantic Health
             </h4>
             <div className="flex items-center justify-center py-6">
                <div className="size-40 rounded-full border-[14px] border-[#1e343b] relative flex items-center justify-center shadow-2xl shadow-primary/10">
                   <div className="absolute inset-0 rounded-full border-[14px] border-primary border-t-transparent border-l-transparent -rotate-45" style={{ transform: `rotate(${(data.health/100) * 360 - 90}deg)` }}></div>
                   <div className="text-center">
                      <span className="block text-4xl font-black text-white leading-none mb-1">{data.health}%</span>
                      <span className="block text-[10px] uppercase font-black text-text-secondary tracking-[0.15em]">Stable</span>
                   </div>
                </div>
             </div>
             <div className="flex justify-between border-t border-[#233f48] pt-8">
                <div className="text-center">
                   <p className="text-white text-xl font-black font-mono leading-none">{data.stats.total}</p>
                   <p className="text-[10px] font-black text-text-secondary mt-2 uppercase tracking-widest">Total</p>
                </div>
                <div className="text-center">
                   <p className="text-red-400 text-xl font-black font-mono leading-none">{data.stats.conflicts}</p>
                   <p className="text-[10px] font-black text-text-secondary mt-2 uppercase tracking-widest">Conflicts</p>
                </div>
                <div className="text-center">
                   <p className="text-orange-400 text-xl font-black font-mono leading-none">{data.stats.stale}</p>
                   <p className="text-[10px] font-black text-text-secondary mt-2 uppercase tracking-widest">Stale</p>
                </div>
             </div>
          </div>
        </div>

        <div className="lg:col-span-8 flex flex-col gap-6">
           <div className="bg-surface-dark border border-[#233f48] rounded-2xl overflow-hidden flex flex-col shadow-2xl flex-1">
              <div className="px-8 py-5 border-b border-[#233f48] flex justify-between items-center bg-black/10">
                 <div className="flex items-center gap-3">
                    <span className="material-symbols-outlined text-red-500">warning</span>
                    <h3 className="text-white text-lg font-bold uppercase tracking-tight">Conflict Queue</h3>
                 </div>
                 <span className="bg-red-500/10 text-red-400 border border-red-500/20 px-4 py-1.5 rounded-lg text-[10px] font-black uppercase tracking-[0.2em]">{data.conflicts.length} Pending Resolution</span>
              </div>
              <div className="p-8 flex-1 overflow-y-auto space-y-8">
                 {data.conflicts.map((conflict: any) => (
                    <div key={conflict.id} className="p-8 rounded-2xl bg-[#111e22]/50 border border-[#233f48] space-y-8 shadow-inner">
                       <div className="flex justify-between items-center">
                          <div className="flex items-center gap-4">
                             <div className="bg-primary/10 p-2.5 rounded-xl text-primary border border-primary/20 shadow-lg"><span className="material-symbols-outlined text-xl">compare</span></div>
                             <div>
                                <p className="text-[10px] font-black text-text-secondary uppercase tracking-[0.2em] opacity-60">Entity Namespace Clash</p>
                                <p className="text-sm font-black text-white uppercase tracking-tight">{conflict.title}</p>
                             </div>
                          </div>
                          <div className="text-right">
                             <p className="text-[10px] font-black text-text-secondary uppercase tracking-widest mb-1">Vector Similarity</p>
                             <span className="text-xs font-mono font-black text-primary bg-primary/10 px-3 py-1 rounded-md border border-primary/20">{conflict.similarity * 100}% Match</span>
                          </div>
                       </div>
                       
                       <div className="grid grid-cols-1 md:grid-cols-2 gap-8 relative">
                          <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-10 size-10 bg-[#101d22] border-2 border-[#233f48] rounded-full flex items-center justify-center text-red-400 font-black text-xs hidden md:flex shadow-2xl">VS</div>
                          <div className="p-6 rounded-2xl bg-surface-dark border border-[#233f48] relative group hover:border-white/20 transition-all">
                             <span className="absolute -top-3 left-4 bg-[#111e22] px-3 py-0.5 border border-[#233f48] rounded-full text-[9px] font-black text-text-secondary uppercase tracking-widest">Memory A (Archived)</span>
                             <p className="text-slate-300 text-sm italic font-medium leading-relaxed">"{conflict.memA.content}"</p>
                             <div className="mt-4 flex justify-between items-center opacity-40">
                                <span className="text-[9px] font-mono font-bold text-text-secondary">{conflict.memA.id}</span>
                                <span className="text-[9px] font-mono font-bold text-text-secondary">{conflict.memA.time}</span>
                             </div>
                          </div>
                          <div className="p-6 rounded-2xl bg-surface-dark border border-primary/30 relative shadow-2xl shadow-primary/5 group hover:border-primary/60 transition-all">
                             <span className="absolute -top-3 left-4 bg-[#111e22] px-3 py-0.5 border border-primary/30 rounded-full text-[9px] font-black text-primary uppercase tracking-widest">Memory B (Active)</span>
                             <p className="text-slate-100 text-sm italic font-black leading-relaxed">"{conflict.memB.content}"</p>
                             <div className="mt-4 flex justify-between items-center opacity-60">
                                <span className="text-[9px] font-mono font-black text-primary">{conflict.memB.id}</span>
                                <span className="text-[9px] font-mono font-black text-primary">{conflict.memB.time}</span>
                             </div>
                          </div>
                       </div>

                       <div className="flex justify-end gap-4">
                          <button onClick={() => handleResolve('Archive')} className="h-10 px-6 rounded-xl border border-[#233f48] text-text-secondary text-xs font-black uppercase tracking-widest hover:bg-white/5 transition-all">Discard New</button>
                          <button onClick={() => handleResolve('Override')} className="h-10 px-6 rounded-xl bg-primary/10 border border-primary/30 text-primary text-xs font-black uppercase tracking-widest hover:bg-primary hover:text-[#101d22] transition-all">Overwrite Stale</button>
                          <button onClick={() => handleResolve('Synthesis')} className="h-10 px-8 rounded-xl bg-white text-[#101d22] text-xs font-black uppercase tracking-widest hover:scale-[1.02] active:scale-[0.98] transition-all shadow-xl flex items-center gap-2">
                             <span className="material-symbols-outlined text-lg">auto_fix_high</span> Synthesize
                          </button>
                       </div>
                    </div>
                 ))}
              </div>
           </div>
        </div>
      </div>
    </div>
  );
};

export default Maintenance;
