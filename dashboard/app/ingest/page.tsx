'use client';

import { useApp } from '@/components/AppProvider';

export default function Ingest() {
  const { showToast } = useApp();

  const handleCommitBatch = () => {
    showToast('Batch knowledge successfully ingested and indexed');
  };

  return (
    <div className="p-6 md:p-10 max-w-[1440px] mx-auto w-full flex flex-col gap-10 h-full overflow-y-auto animate-page-in">
      <div className="max-w-3xl">
        <h1 className="text-4xl font-black text-white tracking-tight mb-4">Ingest Knowledge</h1>
        <p className="text-text-secondary text-lg">Drag and drop your documents to expand the AI&apos;s long-term memory. System processes files locally for maximum privacy.</p>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-12 gap-10">
        <div className="xl:col-span-7 flex flex-col gap-8">
          <div className="relative group cursor-pointer">
            <div className="absolute -inset-1 bg-gradient-to-r from-primary/20 to-purple-500/20 rounded-2xl blur group-hover:opacity-100 transition duration-500 opacity-50"></div>
            <div className="relative bg-surface-dark border-2 border-dashed border-[#325a67] rounded-2xl flex flex-col items-center justify-center p-20 gap-6 group-hover:border-primary/50 transition-all">
              <div className="size-20 rounded-full bg-primary/10 flex items-center justify-center text-primary mb-2">
                <span className="material-symbols-outlined text-4xl">cloud_upload</span>
              </div>
              <div className="text-center">
                <h3 className="text-white text-xl font-bold mb-2">Drop files to upload</h3>
                <p className="text-text-secondary text-sm">PDF, TXT, MD, JSON. Max size 50MB per file.</p>
              </div>
              <div className="flex gap-3">
                <button className="bg-primary hover:bg-primary-dark text-[#101d22] font-black px-6 h-10 rounded-lg shadow-lg shadow-primary/20 transition-all">Browse Files</button>
                <button className="bg-[#101d22] hover:bg-[#1a2c35] text-white border border-[#233f48] px-6 h-10 rounded-lg transition-all font-bold text-sm">Import URL</button>
              </div>
            </div>
          </div>

          <div className="bg-surface-dark rounded-xl border border-[#233f48] p-6 space-y-8">
            <h3 className="text-white text-lg font-bold flex items-center gap-3"><span className="material-symbols-outlined text-primary">tune</span> Batch Settings</h3>
            <div className="space-y-6">
              <div className="space-y-3">
                <label className="text-sm font-bold text-white uppercase tracking-widest text-xs opacity-70">Batch Labels</label>
                <div className="relative">
                  <input className="w-full bg-[#101d22] border border-[#233f48] text-white rounded-lg p-3.5 pr-40 text-sm focus:ring-1 focus:ring-primary outline-none" defaultValue="project-alpha, documentation" />
                  <div className="absolute right-3 top-1/2 -translate-y-1/2 flex gap-1.5">
                    <span className="bg-primary/20 text-primary text-[10px] font-bold px-2 py-0.5 rounded border border-primary/20">project-alpha</span>
                    <span className="bg-primary/20 text-primary text-[10px] font-bold px-2 py-0.5 rounded border border-primary/20">docs</span>
                  </div>
                </div>
              </div>
              <div className="flex items-center justify-between p-4 bg-[#101d22] rounded-xl border border-[#233f48]">
                <div className="flex flex-col gap-1">
                  <p className="text-white text-sm font-bold flex items-center gap-2">Auto-tagging AI <span className="material-symbols-outlined text-primary text-sm">auto_awesome</span></p>
                  <p className="text-text-secondary text-xs">Automatically extract entities and generate semantic tags.</p>
                </div>
                <div className="w-11 h-6 bg-primary rounded-full p-1"><div className="size-4 bg-white rounded-full translate-x-5"></div></div>
              </div>
            </div>
          </div>
        </div>

        <div className="xl:col-span-5 flex flex-col gap-6">
          <div className="bg-surface-dark rounded-2xl border border-[#233f48] overflow-hidden h-full flex flex-col shadow-2xl">
            <div className="px-6 py-5 border-b border-[#233f48] flex justify-between items-center bg-[#192d33]">
              <h3 className="text-white font-bold">Processing Queue</h3>
              <span className="bg-[#233f48] text-text-secondary px-2 py-0.5 rounded-full text-[10px] font-bold">3 items</span>
            </div>
            <div className="flex-1 overflow-y-auto">
              <div className="p-5 border-b border-[#233f48] hover:bg-white/5 cursor-pointer">
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center gap-3">
                    <div className="size-9 rounded bg-red-400/20 text-red-400 flex items-center justify-center"><span className="material-symbols-outlined">picture_as_pdf</span></div>
                    <div>
                      <p className="text-white text-sm font-bold">architecture_v2.pdf</p>
                      <p className="text-text-secondary text-xs">2.4 MB</p>
                    </div>
                  </div>
                  <span className="text-primary text-[10px] font-bold animate-pulse">EXTRACTING...</span>
                </div>
                <div className="w-full bg-[#233f48] h-1.5 rounded-full overflow-hidden">
                  <div className="h-full bg-primary" style={{ width: '65%' }}></div>
                </div>
              </div>
              <div className="p-5 border-b border-[#233f48] bg-primary/5 border-l-4 border-primary">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className="size-9 rounded bg-blue-400/20 text-blue-400 flex items-center justify-center"><span className="material-symbols-outlined">description</span></div>
                    <div>
                      <p className="text-white text-sm font-bold">agent_protocols.md</p>
                      <p className="text-text-secondary text-xs">128 KB</p>
                    </div>
                  </div>
                  <span className="text-green-400 flex items-center gap-1 text-[10px] font-bold"><span className="material-symbols-outlined text-sm">check_circle</span> READY</span>
                </div>
              </div>
            </div>
            <div className="p-6 bg-[#192d33] border-t border-[#233f48] space-y-4">
              <div className="bg-[#111e22] rounded-lg p-4 border border-white/5 space-y-3">
                <div className="flex justify-between items-center">
                  <p className="text-[10px] font-bold text-text-secondary uppercase tracking-widest">Preview: agent_protocols.md</p>
                  <button className="text-primary text-[10px] font-bold uppercase">Edit</button>
                </div>
                <div className="flex flex-wrap gap-2">
                  <span className="bg-purple-500/10 border border-purple-500/20 text-purple-400 text-[10px] font-bold px-2 py-0.5 rounded flex items-center gap-1.5"><span className="material-symbols-outlined text-xs">person</span> Agent Alpha</span>
                  <span className="bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-[10px] font-bold px-2 py-0.5 rounded flex items-center gap-1.5"><span className="material-symbols-outlined text-xs">location_on</span> Core Lab</span>
                </div>
              </div>
              <div className="flex gap-4">
                <button className="flex-1 h-11 border border-[#325a67] text-white rounded-xl font-bold text-sm hover:bg-[#233f48] transition-all">Clear</button>
                <button
                  onClick={handleCommitBatch}
                  className="flex-[2] h-11 bg-primary hover:bg-primary-dark text-[#101d22] rounded-xl font-black text-sm shadow-xl shadow-primary/20 flex items-center justify-center gap-2 transition-all"
                >
                  Commit to Memory <span className="material-symbols-outlined text-lg">arrow_forward</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
