'use client';

export default function GraphView() {
  return (
    <div className="flex flex-col h-full overflow-hidden animate-page-in">
      <header className="flex-none flex items-center justify-between border-b border-[#233f48] bg-background-dark/95 backdrop-blur-md px-6 py-4 z-40">
        <div className="flex items-center gap-8">
          <div className="flex items-center gap-3 text-white">
            <div className="size-8 rounded-lg bg-primary/15 flex items-center justify-center text-primary border border-primary/20">
              <span className="material-symbols-outlined text-xl">hub</span>
            </div>
            <h2 className="text-lg font-black uppercase tracking-tighter">Graph Explorer</h2>
          </div>
          <div className="relative group hidden lg:block">
            <span className="material-symbols-outlined absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary text-lg">search</span>
            <input className="bg-surface-dark border border-[#233f48] rounded-lg h-9 w-64 pl-10 pr-4 text-xs text-white focus:ring-1 focus:ring-primary focus:border-primary transition-all" placeholder="Search knowledge nodes..." />
          </div>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-[10px] font-black text-text-secondary uppercase tracking-widest">Layout: Force-Directed</span>
        </div>
      </header>

      <div className="flex-1 flex overflow-hidden">
        {/* Left Controls */}
        <aside className="w-80 border-r border-[#233f48] bg-surface-dark flex flex-col z-30 shadow-2xl">
          <div className="p-6 border-b border-[#233f48] bg-black/10">
            <h2 className="text-white text-[10px] font-black uppercase tracking-[0.2em] mb-4">Nav Mode</h2>
            <div className="flex h-11 bg-[#101d22] p-1.5 rounded-xl border border-[#233f48]">
              <button className="flex-1 rounded-lg bg-primary text-[#101d22] text-[10px] font-black uppercase tracking-widest flex items-center justify-center gap-2 shadow-lg shadow-primary/20">
                <span className="material-symbols-outlined text-sm font-bold">explore</span> Explore
              </button>
              <button className="flex-1 rounded-lg text-text-secondary text-[10px] font-black uppercase tracking-widest flex items-center justify-center gap-2 hover:text-white transition-colors">
                <span className="material-symbols-outlined text-sm">route</span> Path
              </button>
            </div>
          </div>
          <div className="p-6 flex-1 space-y-10 overflow-y-auto">
            <div className="space-y-5">
              <h2 className="text-white text-[10px] font-black uppercase tracking-[0.2em]">Filter Labels</h2>
              <div className="space-y-3">
                {[
                  { label: 'Agent', color: 'bg-primary', count: 24 },
                  { label: 'Location', color: 'bg-purple-500', count: 12 },
                  { label: 'Objective', color: 'bg-orange-500', count: 8 },
                  { label: 'Concept', color: 'bg-emerald-500', count: 45 }
                ].map(type => (
                  <label key={type.label} className="flex items-center gap-3 group cursor-pointer">
                    <div className="relative size-4 rounded bg-[#101d22] border border-[#233f48] group-hover:border-primary/50 transition-colors flex items-center justify-center">
                      <input type="checkbox" defaultChecked className="absolute inset-0 opacity-0 cursor-pointer z-10" />
                      <span className="material-symbols-outlined text-primary text-[14px] opacity-100 font-black">check</span>
                    </div>
                    <span className={`size-2 rounded-full ${type.color} shadow-sm`}></span>
                    <span className="text-text-secondary group-hover:text-white text-xs font-bold transition-colors">{type.label}</span>
                    <span className="ml-auto text-[10px] text-text-secondary font-mono opacity-40">{type.count}</span>
                  </label>
                ))}
              </div>
            </div>
            <div className="space-y-5">
              <h2 className="text-white text-[10px] font-black uppercase tracking-[0.2em]">Pruning Threshold</h2>
              <input type="range" className="w-full h-1.5 bg-[#101d22] rounded-full appearance-none accent-primary border border-[#233f48] cursor-pointer" defaultValue="65" />
              <div className="flex justify-between mt-2 text-[10px] text-text-secondary font-black font-mono">
                <span>0.0</span>
                <span className="text-primary">0.65</span>
                <span>1.0</span>
              </div>
            </div>
          </div>
          <div className="p-6 border-t border-[#233f48] bg-black/10">
            <button className="w-full h-12 bg-primary hover:bg-primary-dark text-[#101d22] font-black uppercase tracking-[0.15em] py-2.5 rounded-xl shadow-xl shadow-primary/20 flex items-center justify-center gap-2 transition-all active:scale-95 text-xs">
              <span className="material-symbols-outlined text-lg">sync</span> Redraw Graph
            </button>
          </div>
        </aside>

        {/* Graph Canvas */}
        <div className="flex-1 relative bg-[#0a1418] graph-grid-bg cursor-grab active:cursor-grabbing">
          <div className="absolute inset-0 flex items-center justify-center overflow-hidden">
            <svg className="absolute inset-0 w-full h-full opacity-30" xmlns="http://www.w3.org/2000/svg">
              <line x1="50%" y1="50%" x2="35%" y2="35%" stroke="#13b6ec" strokeWidth="1" strokeDasharray="4 4" />
              <line x1="50%" y1="50%" x2="65%" y2="25%" stroke="#233f48" strokeWidth="1" />
              <line x1="50%" y1="50%" x2="70%" y2="60%" stroke="#13b6ec" strokeWidth="2" />
            </svg>

            {/* Central Selected Node */}
            <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-20 flex flex-col items-center gap-4">
              <div className="size-20 bg-[#0a1418] rounded-2xl border-2 border-primary shadow-[0_0_50px_rgba(19,182,236,0.3)] flex items-center justify-center node-pulse hover:scale-110 transition-transform cursor-pointer relative">
                <span className="material-symbols-outlined text-primary text-4xl">smart_toy</span>
                <div className="absolute -top-2 -right-2 size-6 bg-red-500 rounded-lg border-2 border-[#0a1418] text-[10px] font-black text-white flex items-center justify-center shadow-lg">3</div>
              </div>
              <div className="bg-surface-dark px-4 py-1.5 rounded-xl border border-primary/30 text-white text-[10px] font-black uppercase tracking-widest shadow-2xl whitespace-nowrap">Agent Alpha</div>
            </div>

            {/* Secondary Nodes */}
            <div className="absolute top-[35%] left-[35%] flex flex-col items-center gap-3 group cursor-pointer z-10">
              <div className="size-14 bg-[#0a1418] rounded-xl border-2 border-purple-500 flex items-center justify-center hover:scale-110 transition-transform group-hover:shadow-[0_0_30px_rgba(168,85,247,0.3)]">
                <span className="material-symbols-outlined text-purple-500 text-2xl">location_on</span>
              </div>
              <div className="opacity-0 group-hover:opacity-100 transition-opacity bg-surface-dark px-3 py-1 rounded-lg border border-[#233f48] text-[9px] font-bold text-white uppercase tracking-widest">Safehouse 4</div>
            </div>
          </div>

          {/* View Controls */}
          <div className="absolute bottom-8 right-8 flex flex-col gap-3">
            <div className="bg-surface-dark rounded-xl border border-[#233f48] overflow-hidden flex flex-col shadow-2xl">
              <button className="size-12 hover:bg-primary/10 hover:text-primary text-text-secondary transition-all border-b border-[#233f48] flex items-center justify-center active:bg-primary/20"><span className="material-symbols-outlined">add</span></button>
              <button className="size-12 hover:bg-primary/10 hover:text-primary text-text-secondary transition-all flex items-center justify-center active:bg-primary/20"><span className="material-symbols-outlined">remove</span></button>
            </div>
            <button className="size-12 bg-surface-dark rounded-xl border border-[#233f48] hover:bg-primary/10 hover:text-primary text-text-secondary flex items-center justify-center shadow-2xl transition-all active:scale-95"><span className="material-symbols-outlined">center_focus_strong</span></button>
          </div>
        </div>

        {/* Right Inspector */}
        <aside className="w-[400px] border-l border-[#233f48] bg-surface-dark p-8 overflow-y-auto z-30 shadow-2xl">
          <div className="flex justify-between items-start mb-10">
            <div>
              <h2 className="text-white text-xl font-black uppercase tracking-tight">Inspector</h2>
              <p className="text-text-secondary text-[10px] font-mono mt-1.5 opacity-60">ID: 8f92-a1b2-c3d4</p>
            </div>
            <button className="text-text-secondary hover:text-white transition-colors h-8 w-8 rounded-lg hover:bg-[#233f48] flex items-center justify-center"><span className="material-symbols-outlined">close</span></button>
          </div>

          <div className="space-y-10">
            <div className="bg-black/20 rounded-2xl p-6 border border-[#233f48] shadow-inner">
              <div className="flex items-center gap-5 mb-6">
                <div className="size-14 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary shadow-lg">
                  <span className="material-symbols-outlined text-3xl font-bold">smart_toy</span>
                </div>
                <div>
                  <h3 className="text-white font-black text-lg leading-none uppercase tracking-tight">Agent Alpha</h3>
                  <div className="flex gap-2 mt-2.5">
                    <span className="bg-primary text-[#101d22] text-[9px] font-black px-2 py-0.5 rounded-md uppercase tracking-wider">Master Entity</span>
                    <span className="bg-green-500/10 text-green-400 border border-green-500/20 text-[9px] font-black px-2 py-0.5 rounded-md uppercase tracking-wider">Verified</span>
                  </div>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="bg-[#1a2c35] p-4 rounded-xl border border-[#233f48]">
                  <p className="text-[9px] text-text-secondary uppercase font-black tracking-[0.1em] mb-2 opacity-60">Recall Score</p>
                  <p className="text-white text-2xl font-black font-mono leading-none tracking-tighter">98.5%</p>
                </div>
                <div className="bg-[#1a2c35] p-4 rounded-xl border border-[#233f48]">
                  <p className="text-[9px] text-text-secondary uppercase font-black tracking-[0.1em] mb-2 opacity-60">Connections</p>
                  <p className="text-white text-2xl font-black font-mono leading-none tracking-tighter">1,240</p>
                </div>
              </div>
            </div>

            <div className="space-y-5">
              <h3 className="text-white text-[10px] font-black uppercase tracking-[0.2em] flex items-center gap-2"><span className="material-symbols-outlined text-sm text-primary">description</span> Raw Payload</h3>
              <div className="bg-[#0f1619] p-5 rounded-xl border border-[#233f48] font-mono text-[11px] text-slate-400 leading-relaxed overflow-x-auto whitespace-pre shadow-inner">
{`{
  "system_id": "alpha_01",
  "priority": "omega",
  "status": "active_monitoring",
  "last_check": "2024-05-12T04:22:11Z"
}`}
              </div>
            </div>

            <div className="space-y-5">
              <h3 className="text-white text-[10px] font-black uppercase tracking-[0.2em] flex items-center gap-2"><span className="material-symbols-outlined text-sm text-primary">link</span> Direct Relations</h3>
              <div className="space-y-3">
                {[
                  { label: 'Controlled By', target: 'Root Admin', icon: 'security', color: 'text-blue-400' },
                  { label: 'Deploys To', target: 'Safehouse 4', icon: 'location_on', color: 'text-purple-400' },
                  { label: 'Conflict With', target: 'Agent Beta', icon: 'warning', color: 'text-red-400' }
                ].map(rel => (
                  <div key={rel.target} className="flex items-center justify-between p-4 rounded-xl bg-black/10 border border-[#233f48] group hover:border-primary/50 cursor-pointer transition-all active:scale-[0.98]">
                    <div className="flex items-center gap-4">
                      <span className={`material-symbols-outlined text-xl ${rel.color}`}>{rel.icon}</span>
                      <div>
                        <p className="text-white text-xs font-black uppercase tracking-tight">{rel.target}</p>
                        <p className="text-[9px] text-text-secondary mt-1 font-bold uppercase opacity-50">{rel.label}</p>
                      </div>
                    </div>
                    <span className="material-symbols-outlined text-text-secondary text-sm group-hover:text-primary transition-colors">arrow_forward_ios</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
}
