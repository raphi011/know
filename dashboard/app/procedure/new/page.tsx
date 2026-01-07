'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { useApp } from '@/components/AppProvider';
import { graphql, MUTATIONS } from '@/lib/graphql';
import { ProcedureStep } from '@/lib/types';

export default function NewProcedure() {
  const router = useRouter();
  const { showToast, currentContext } = useApp();

  const [formData, setFormData] = useState({
    name: '',
    description: '',
    steps: [{ content: '' }] as ProcedureStep[],
    labels: [] as string[],
    context: currentContext === 'all' ? 'memcp' : currentContext,
  });

  const addStep = () => {
    setFormData({ ...formData, steps: [...formData.steps, { content: '' }] });
  };

  const removeStep = (idx: number) => {
    if (formData.steps.length === 1) return;
    const nextSteps = formData.steps.filter((_, i) => i !== idx);
    setFormData({ ...formData, steps: nextSteps });
  };

  const updateStep = (idx: number, content: string) => {
    const nextSteps = [...formData.steps];
    nextSteps[idx].content = content;
    setFormData({ ...formData, steps: nextSteps });
  };

  const handleSave = async () => {
    if (!formData.name.trim()) return showToast('Name is required', 'error');
    try {
      await graphql(MUTATIONS.saveProcedure, { procedure: formData });
      showToast('Procedure saved successfully');
      router.push('/procedures');
    } catch {
      showToast('Failed to save procedure', 'error');
    }
  };

  return (
    <div className="flex flex-col h-full animate-page-in overflow-hidden">
      <header className="flex items-center justify-between px-6 py-4 border-b border-[#233f48] bg-background-dark/80 backdrop-blur-md sticky top-0 z-30">
        <div className="flex items-center gap-4">
          <Link href="/procedures" className="size-10 rounded-xl bg-surface-dark border border-[#233f48] flex items-center justify-center text-text-secondary hover:text-white transition-all">
            <span className="material-symbols-outlined">close</span>
          </Link>
          <h2 className="text-white text-xl font-black leading-none uppercase tracking-tight">
            New Procedure Protocol
          </h2>
        </div>
        <div className="flex items-center gap-3">
          <button onClick={handleSave} className="bg-primary hover:bg-primary-dark text-[#101d22] font-black px-8 py-2.5 rounded-xl text-xs uppercase tracking-[0.1em] shadow-xl shadow-primary/20 transition-all">
            Commit Protocol
          </button>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto p-8 md:p-12">
        <div className="max-w-4xl mx-auto space-y-10">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
            <div className="space-y-3">
              <label className="text-[10px] font-black text-text-secondary uppercase tracking-[0.2em]">Procedure Name</label>
              <input
                className="w-full bg-surface-dark border border-[#233f48] rounded-xl p-4 text-white focus:ring-1 focus:ring-primary outline-none"
                placeholder="e.g. Deploy to Production"
                value={formData.name}
                onChange={e => setFormData({ ...formData, name: e.target.value })}
              />
            </div>
            <div className="space-y-3">
              <label className="text-[10px] font-black text-text-secondary uppercase tracking-[0.2em]">Namespace Context</label>
              <input
                className="w-full bg-surface-dark border border-[#233f48] rounded-xl p-4 text-white font-mono focus:ring-1 focus:ring-primary outline-none"
                value={formData.context}
                onChange={e => setFormData({ ...formData, context: e.target.value })}
              />
            </div>
          </div>

          <div className="space-y-3">
            <label className="text-[10px] font-black text-text-secondary uppercase tracking-[0.2em]">Description / Purpose</label>
            <textarea
              className="w-full bg-surface-dark border border-[#233f48] rounded-xl p-4 text-white focus:ring-1 focus:ring-primary outline-none resize-none h-24"
              placeholder="What does this procedure achieve?"
              value={formData.description}
              onChange={e => setFormData({ ...formData, description: e.target.value })}
            ></textarea>
          </div>

          <div className="space-y-6">
            <div className="flex items-center justify-between border-b border-[#233f48] pb-4">
              <h3 className="text-white text-sm font-black uppercase tracking-widest flex items-center gap-3">
                <span className="material-symbols-outlined text-primary">format_list_numbered</span> Step Builder
              </h3>
              <button onClick={addStep} className="text-primary text-[10px] font-black uppercase tracking-widest flex items-center gap-2 hover:bg-primary/10 px-3 py-1.5 rounded-lg transition-all">
                <span className="material-symbols-outlined text-sm">add</span> Add Step
              </button>
            </div>

            <div className="space-y-4">
              {formData.steps.map((step, i) => (
                <div key={i} className="flex gap-4 items-start group animate-page-in">
                  <div className="size-10 rounded-xl bg-surface-dark border border-[#233f48] flex items-center justify-center text-primary font-mono font-black text-xs shrink-0 group-hover:border-primary/50 transition-colors">
                    {i + 1}
                  </div>
                  <div className="flex-1">
                    <textarea
                      className="w-full bg-surface-dark border border-[#233f48] rounded-xl p-4 text-white text-sm focus:ring-1 focus:ring-primary outline-none h-20"
                      placeholder="Define step instruction..."
                      value={step.content}
                      onChange={e => updateStep(i, e.target.value)}
                    ></textarea>
                  </div>
                  <button
                    onClick={() => removeStep(i)}
                    className="size-10 rounded-xl bg-surface-dark border border-[#233f48] flex items-center justify-center text-text-secondary hover:text-red-400 hover:border-red-400/50 transition-all shrink-0"
                  >
                    <span className="material-symbols-outlined">delete</span>
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
