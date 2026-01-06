
import React, { useEffect, useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { backend } from '../backend';
import { useApp } from '../App';

const EntityDetail = () => {
  const { id } = useParams();
  const { showToast } = useApp();
  const [entity, setEntity] = useState<any>(null);
  const [userImportance, setUserImportance] = useState(0.5);

  useEffect(() => {
    const fetchData = async () => {
      const result = await backend.getEntity(id || '');
      setEntity(result);
      if (result) setUserImportance(result.user_importance || 0.5);
    };
    fetchData();
  }, [id]);

  const handleUpdateImportance = () => {
    showToast('Heuristic importance recalibrated successfully');
  };

  if (!entity) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-background-dark text-text-secondary">
        <div className="size-8 border-2 border-primary border-t-transparent rounded-full animate-spin"></div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full bg-background-dark overflow-y-auto animate-page-in">
      <header className="flex-none flex items-center justify-between border-b border-[#233f48] bg-background-dark/95 backdrop-blur-md px-6 py-4 sticky top-0 z-40">
        <div className="flex items-center gap-8">
           <nav className="flex items-center text-[10px] font-black uppercase tracking-[0.15em] text-text-secondary">
            <Link to="/" className="hover:text-primary transition-colors">MEMCP</Link>
            <span className="material-symbols-outlined text-[14px] mx-3 opacity-30">chevron_right</span>
            <Link to="/search" className="hover:text-primary transition-colors">STORAGE</Link>
            <span className="material-symbols-outlined text-[14px] mx-3 opacity-30">chevron_right</span>
            <span className="text-white font-mono lowercase tracking-normal bg-[#233f48] px-2 py-0.5 rounded">{entity.id}</span>
          </nav>
        </div>
        <div className="flex items-center gap-4">
          <div className="flex rounded-xl bg-surface-dark p-1 border border-[#233f48] shadow-sm">
            <button className="p-2.5 text-text-secondary hover:text-white transition-colors rounded-lg hover:bg-white/5"><span className="material-symbols-outlined text-[20px]">edit</span></button>
            <button className="p-2.5 text-text-secondary hover:text-red-400 transition-colors rounded-lg hover:bg-red-400/5"><span className="material-symbols-outlined text-[20px]">delete</span></button>
          </div>
          <button className="flex items-center gap-3 px-6 py-2.5 bg-primary hover:bg-primary-dark text-[#111e22] font-black rounded-xl text-xs uppercase tracking-widest shadow-xl shadow-primary/20 transition-all active:scale-95">
            <span className="material-symbols-outlined text-[18px]">verified_user</span> Integrity Check
          </button>
        </div>
      </header>

      <main className="p-8 md:p-12 max-w-[1200px] mx-auto w-full grid grid-cols-1 lg:grid-cols-12 gap-10">
        <div className="lg:col-span-8 flex flex-col gap-10">
          <div className="flex flex-col gap-3 border-b border-[#233f48] pb-8">
            <div className="flex items-center gap-4">
              <h1 className="text-4xl font-black tracking-tighter text-white font-mono uppercase">{entity.id}</h1>
              <span className="bg-primary/15 px-3 py-1 rounded-lg text-[10px] font-black text-primary border border-primary/20 uppercase tracking-[0.2em]">Verified {entity.type}</span>
            </div>
            <p className="text-text-secondary text-xs flex items-center gap-2 font-bold uppercase tracking-widest opacity-60">
              <span className="material-symbols-outlined text-[16px]">schedule</span> Entry created {entity.lastAccessed} • {entity.accessCount} accesses
            </p>
          </div>

          <section className="bg-surface-dark rounded-2xl border border-[#233f48] overflow-hidden flex flex-col shadow-2xl relative">
            <div className="px-8 py-5 border-b border-[#233f48] flex justify-between items-center bg-black/10">
              <h3 className="font-black text-white text-xs uppercase tracking-[0.2em] flex items-center gap-3">
                <span className="material-symbols-outlined text-primary text-[20px]">description</span> Memory Content
              </h3>
              <div className="flex gap-4 text-[10px] font-black uppercase tracking-widest">
                <button className="text-text-secondary hover:text-white transition-colors">Raw JSON</button>
                <button className="text-primary underline underline-offset-4 decoration-primary/40">Parsed Text</button>
              </div>
            </div>
            <div className="p-10 space-y-8">
              <p className="text-2xl leading-relaxed text-slate-100 font-medium">
                {entity.content}
              </p>
              <div className="p-6 rounded-2xl bg-black/30 border border-[#233f48] font-mono text-sm overflow-x-auto text-slate-400 shadow-inner">
                <pre className="leading-relaxed">{JSON.stringify({
                  id: entity.id,
                  type: entity.type,
                  importance: entity.importance,
                  context: entity.context,
                  tags: entity.labels
                }, null, 2)}</pre>
              </div>
            </div>
          </section>

          <div className="space-y-6">
            <h3 className="font-black text-white text-sm uppercase tracking-[0.2em] flex items-center gap-3">
              <span className="material-symbols-outlined text-primary text-xl">link</span> Semantic Neighbors
            </h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
              {entity.neighbors.map((neighbor: any, idx: number) => (
                <div key={idx} className={`p-6 rounded-2xl border border-[#233f48] bg-surface-dark transition-all cursor-pointer group shadow-sm active:scale-[0.98] ${
                  neighbor.type === 'Contradicts' ? 'hover:border-red-400/50' : 'hover:border-primary/50'
                }`}>
                  <div className="flex justify-between items-start mb-4">
                    <span className={`text-[10px] font-black uppercase tracking-widest flex items-center gap-2 ${
                      neighbor.type === 'Contradicts' ? 'text-red-400' : 'text-green-400'
                    }`}>
                      <span className={`size-1.5 rounded-full shadow-[0_0_8px_rgba(74,222,128,0.5)] ${
                        neighbor.type === 'Contradicts' ? 'bg-red-400' : 'bg-green-400'
                      }`}></span> {neighbor.type}
                    </span>
                    <span className="text-[9px] font-mono font-bold text-text-secondary opacity-40">{neighbor.id}</span>
                  </div>
                  <p className={`text-sm font-bold text-slate-300 transition-colors leading-relaxed ${
                    neighbor.type === 'Contradicts' ? 'group-hover:text-red-400' : 'group-hover:text-primary'
                  }`}>{neighbor.content}</p>
                  <div className="mt-4 text-[9px] text-text-secondary font-black uppercase tracking-widest opacity-50">Similarity Score: {neighbor.score}</div>
                </div>
              ))}
            </div>
          </div>
        </div>

        <aside className="lg:col-span-4 flex flex-col gap-8">
          <div className="bg-surface-dark rounded-2xl border border-[#233f48] p-8 space-y-8 shadow-2xl relative">
             <h4 className="font-black text-white uppercase text-[10px] tracking-[0.2em] border-b border-[#233f48] pb-5">Attributes</h4>
             
             <div className="space-y-4">
                <span className="text-[10px] font-black text-text-secondary uppercase tracking-[0.1em]">Knowledge Labels</span>
                <div className="flex flex-wrap gap-2.5">
                  {entity.labels.map((l: string) => (
                    <span key={l} className="bg-black/20 border border-[#233f48] text-slate-300 px-3 py-1 rounded-lg text-[10px] font-black uppercase tracking-widest flex items-center gap-2.5 hover:border-primary/50 transition-colors group cursor-default">
                      {l} <button className="text-text-secondary hover:text-red-400 transition-colors"><span className="material-symbols-outlined text-[14px]">close</span></button>
                    </span>
                  ))}
                  <button className="size-7 border border-dashed border-[#233f48] rounded-lg flex items-center justify-center text-text-secondary hover:border-white hover:text-white transition-all"><span className="material-symbols-outlined text-[16px]">add</span></button>
                </div>
             </div>

             <div className="p-6 bg-black/30 rounded-2xl border border-[#233f48] space-y-6 shadow-inner">
                <div className="space-y-3">
                   <div className="flex justify-between items-center text-[10px] uppercase font-black tracking-widest">
                      <span className="text-text-secondary">Importance Heuristic</span>
                      <span className="text-primary font-mono font-black">{entity.importance}</span>
                   </div>
                   <div className="h-1.5 w-full bg-[#101d22] rounded-full overflow-hidden">
                      <div className="h-full bg-primary shadow-[0_0_10px_rgba(19,182,236,0.5)]" style={{ width: `${entity.importance * 100}%` }}></div>
                   </div>
                </div>
                
                <div className="space-y-4 pt-2">
                   <div className="flex justify-between items-center text-[9px] font-black text-text-secondary uppercase tracking-[0.15em]">
                      <span>User Importance Override</span>
                      <span className="text-white bg-[#233f48] px-2 py-0.5 rounded">{userImportance.toFixed(2)}</span>
                   </div>
                   <input 
                    type="range" 
                    min="0" max="1" step="0.05"
                    className="w-full h-1.5 bg-[#101d22] rounded-full appearance-none accent-primary cursor-pointer border border-[#233f48]" 
                    value={userImportance}
                    onChange={(e) => setUserImportance(parseFloat(e.target.value))}
                   />
                   <button 
                    onClick={handleUpdateImportance}
                    className="w-full py-2 bg-[#233f48] hover:bg-primary/20 text-[9px] font-black text-white uppercase tracking-widest rounded-lg border border-white/5 transition-all"
                   >
                     Apply Override
                   </button>
                </div>
             </div>

             <div className="space-y-4">
                <span className="text-[10px] font-black text-text-secondary uppercase tracking-[0.1em]">Project Namespace</span>
                <div className="bg-black/20 p-3 rounded-xl border border-[#233f48] flex items-center gap-3">
                   <span className="material-symbols-outlined text-primary text-xl">folder</span>
                   <span className="text-xs font-black text-white uppercase">{entity.context}</span>
                </div>
             </div>
          </div>
        </aside>
      </main>
    </div>
  );
};

export default EntityDetail;
