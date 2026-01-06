
import React, { useState } from 'react';
import { useApp } from '../App';
import { EntityType } from '../types';

const AddMemory = () => {
  const { showToast, currentContext } = useApp();
  const [loadingTags, setLoadingTags] = useState(false);
  const [formData, setFormData] = useState({
    type: EntityType.Observation,
    content: '',
    importance: 0.5,
    labels: [] as string[],
    context: currentContext === 'all' ? 'memcp' : currentContext
  });

  const handleCommit = () => {
    if (!formData.content.trim()) return showToast('Content is required', 'error');
    showToast('Memory committed to persistent storage');
    setFormData({ ...formData, content: '', labels: [] });
  };

  const handleSuggestTags = async () => {
    if (!formData.content.trim()) return showToast('Add content first', 'warning');
    setLoadingTags(true);
    // Mock tag suggestion based on content
    await new Promise(r => setTimeout(r, 500));
    const mockTags = ['auto-tagged', formData.type.toLowerCase()];
    setFormData({ ...formData, labels: Array.from(new Set([...formData.labels, ...mockTags])) });
    showToast('Tags suggested');
    setLoadingTags(false);
  };

  return (
    <div className="p-6 md:p-10 max-w-[1280px] mx-auto w-full flex flex-col gap-8 h-full overflow-y-auto animate-page-in">
      <div className="flex flex-col gap-2 border-b border-[#233f48] pb-6">
        <h1 className="text-3xl font-black text-white tracking-tight uppercase">Append Node</h1>
        <p className="text-text-secondary">Manual ingestion and semantic attribute configuration.</p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-12 gap-10">
        <div className="lg:col-span-7 flex flex-col gap-6">
          <div className="bg-surface-dark rounded-xl border border-[#233f48] overflow-hidden shadow-2xl">
            <h3 className="text-white text-lg font-bold px-6 py-4 border-b border-[#233f48] flex items-center gap-3 bg-black/10">
              <span className="material-symbols-outlined text-primary">data_object</span> Entity Parameters
            </h3>
            <div className="p-8 space-y-8">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                <div className="space-y-3">
                  <label className="text-[10px] font-black text-text-secondary uppercase tracking-[0.2em]">Ontology Type</label>
                  <select 
                    value={formData.type}
                    onChange={(e) => setFormData({...formData, type: e.target.value as EntityType})}
                    className="w-full bg-[#101d22] border border-[#233f48] text-white text-sm rounded-lg p-3 focus:ring-1 focus:ring-primary focus:border-primary outline-none appearance-none cursor-pointer"
                  >
                    {Object.values(EntityType).map(type => <option key={type} value={type}>{type.toUpperCase()}</option>)}
                  </select>
                </div>
                <div className="space-y-3">
                  <label className="text-[10px] font-black text-text-secondary uppercase tracking-[0.2em]">Project Context</label>
                  <input 
                    className="w-full bg-[#101d22] border border-[#233f48] text-white text-sm rounded-lg p-3 font-mono focus:ring-1 focus:ring-primary outline-none" 
                    value={formData.context}
                    onChange={(e) => setFormData({...formData, context: e.target.value})}
                  />
                </div>
              </div>
              
              <div className="space-y-4">
                <div className="flex justify-between items-center">
                  <label className="text-[10px] font-black text-text-secondary uppercase tracking-[0.2em]">Semantic Content</label>
                  <button 
                    onClick={handleSuggestTags}
                    disabled={loadingTags}
                    className="text-[10px] font-black text-primary uppercase tracking-widest flex items-center gap-2 hover:underline disabled:opacity-50"
                  >
                    {loadingTags ? 'AI Analyzing...' : <><span className="material-symbols-outlined text-sm">auto_awesome</span> Suggest Labels</>}
                  </button>
                </div>
                <textarea 
                  rows={6} 
                  className="w-full bg-[#101d22] border border-[#233f48] text-white text-sm rounded-lg p-4 focus:ring-1 focus:ring-primary focus:border-primary outline-none resize-none leading-relaxed" 
                  placeholder="Describe the observation, fact, or preference..."
                  value={formData.content}
                  onChange={(e) => setFormData({...formData, content: e.target.value})}
                ></textarea>
                
                <div className="flex flex-wrap gap-2">
                  {formData.labels.map((tag, i) => (
                    <span key={i} className="px-3 py-1 rounded-md bg-primary/10 border border-primary/20 text-[10px] font-black text-primary uppercase tracking-widest flex items-center gap-2">
                      {tag}
                      <button onClick={() => setFormData({...formData, labels: formData.labels.filter(l => l !== tag)})} className="hover:text-white">
                        <span className="material-symbols-outlined text-xs">close</span>
                      </button>
                    </span>
                  ))}
                </div>
              </div>

              <div className="p-6 bg-black/20 rounded-2xl border border-[#233f48] space-y-4">
                 <div className="flex justify-between items-center">
                    <label className="text-[10px] font-black text-text-secondary uppercase tracking-[0.2em]">Heuristic Importance Override</label>
                    <span className="text-primary font-mono font-black text-lg">{formData.importance.toFixed(2)}</span>
                 </div>
                 <input 
                  type="range" 
                  className="w-full h-2 bg-[#101d22] rounded-full appearance-none accent-primary cursor-pointer border border-[#233f48]" 
                  value={formData.importance} 
                  step="0.01" min="0" max="1" 
                  onChange={(e) => setFormData({...formData, importance: parseFloat(e.target.value)})}
                 />
                 <div className="flex justify-between text-[9px] font-black text-text-secondary opacity-40 uppercase">
                    <span>Low Priority</span>
                    <span>System Critical</span>
                 </div>
              </div>
            </div>
          </div>
        </div>

        <div className="lg:col-span-5 flex flex-col gap-6">
          <div className="bg-surface-dark rounded-xl border border-[#233f48] h-full flex flex-col overflow-hidden shadow-xl">
            <h3 className="text-white text-lg font-bold px-6 py-4 border-b border-[#233f48] flex items-center gap-3 bg-black/10">
              <span className="material-symbols-outlined text-primary">hub</span> Structural Bindings
            </h3>
            <div className="p-8 flex-1 flex flex-col gap-8">
               <div className="space-y-3">
                  <label className="text-[10px] font-black text-text-secondary uppercase tracking-[0.2em]">Source ID</label>
                  <div className="relative">
                    <input className="w-full bg-[#101d22] border border-[#233f48] text-white text-sm rounded-lg p-3 font-mono" readOnly value="AUTO_GENERATED" />
                    <span className="material-symbols-outlined absolute right-3 top-1/2 -translate-y-1/2 text-primary opacity-50">magic_button</span>
                  </div>
               </div>
               <div className="flex flex-col items-center justify-center -my-2 opacity-30 gap-1">
                 <div className="h-6 w-px bg-primary"></div>
                 <span className="text-[9px] font-black uppercase text-primary">Relationship</span>
                 <div className="h-6 w-px bg-primary"></div>
               </div>
               <div className="space-y-3">
                  <select className="w-full bg-primary/10 border-2 border-primary/30 text-primary font-black text-sm rounded-lg p-3 text-center cursor-pointer uppercase tracking-widest hover:bg-primary/20 transition-all outline-none">
                    <option>SUPPORTS</option>
                    <option>CONTRADICTS</option>
                    <option>PART_OF</option>
                    <option>IS_A</option>
                  </select>
               </div>
               <div className="flex flex-col items-center justify-center -my-2 opacity-30 gap-1">
                 <div className="h-6 w-px bg-primary"></div>
                 <div className="h-6 w-px bg-primary"></div>
               </div>
               <div className="space-y-3">
                  <label className="text-[10px] font-black text-text-secondary uppercase tracking-[0.2em]">Target Entity</label>
                  <input className="w-full bg-[#101d22] border border-[#233f48] text-white text-sm rounded-lg p-3" placeholder="ID or Content Search..." />
               </div>
            </div>
          </div>
        </div>
      </div>

      <div className="mt-auto bg-surface-dark/80 backdrop-blur-md p-6 rounded-2xl border border-[#233f48] shadow-2xl flex flex-col md:flex-row justify-between items-center gap-8 border-t-primary/20">
         <div className="flex gap-4">
            <div className="flex items-center gap-3 px-4 py-2 bg-black/20 rounded-xl border border-white/5">
               <span className="size-2 rounded-full bg-primary animate-pulse"></span>
               <span className="text-xs font-bold text-text-secondary uppercase tracking-widest">Auto-Verify Active</span>
            </div>
         </div>
         <div className="flex gap-4 w-full md:w-auto">
            <button 
              onClick={() => setFormData({ ...formData, content: '', labels: [] })}
              className="flex-1 md:flex-none px-8 py-3 rounded-xl border border-[#233f48] text-text-secondary hover:bg-red-400/10 hover:text-red-400 transition-all font-bold text-sm uppercase"
            >
              Reset
            </button>
            <button 
              onClick={handleCommit}
              className="flex-1 md:flex-none px-12 py-3 rounded-xl bg-primary hover:bg-primary-dark text-[#101d22] font-black text-sm shadow-xl shadow-primary/20 transition-all transform hover:scale-[1.02] uppercase tracking-[0.1em]"
            >
              Commit Node
            </button>
         </div>
      </div>
    </div>
  );
};

export default AddMemory;
