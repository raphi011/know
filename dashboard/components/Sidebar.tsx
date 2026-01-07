'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useEffect, useState } from 'react';
import { useApp } from './AppProvider';
import { graphql, QUERIES } from '@/lib/graphql';

function SidebarLink({ href, icon, label }: { href: string; icon: string; label: string }) {
  const pathname = usePathname();
  const isActive = pathname === href || (href !== '/' && pathname.startsWith(href));

  return (
    <Link
      href={href}
      className={`flex items-center gap-3 px-3 py-2.5 rounded-lg group transition-all duration-200 border border-transparent ${
        isActive ? 'bg-primary/15 border-primary/20 shadow-inner' : 'hover:bg-white/5'
      }`}
    >
      <span
        className={`material-symbols-outlined transition-colors duration-200 ${
          isActive ? 'text-primary' : 'text-text-secondary group-hover:text-white'
        }`}
        style={{ fontVariationSettings: isActive ? "'FILL' 1" : "'FILL' 0" }}
      >
        {icon}
      </span>
      <p className={`font-medium text-sm transition-colors duration-200 ${
        isActive ? 'text-primary' : 'text-text-secondary group-hover:text-white'
      }`}>
        {label}
      </p>
    </Link>
  );
}

export function Sidebar() {
  const { currentContext, setContext } = useApp();
  const [contexts, setContexts] = useState<string[]>(['all']);

  useEffect(() => {
    graphql<{ contexts: string[] }>(QUERIES.contexts)
      .then((data) => setContexts(['all', ...data.contexts]))
      .catch(() => setContexts(['all', 'memcp']));
  }, []);

  return (
    <aside className="w-64 bg-background-dark border-r border-[#233f48] flex-shrink-0 flex flex-col justify-between p-4 hidden md:flex z-50">
      <div className="flex flex-col gap-6">
        <div className="flex flex-col px-2">
          <div className="flex items-center gap-2.5 mb-1.5">
            <div className="w-7 h-7 bg-primary rounded-lg flex items-center justify-center shadow-lg shadow-primary/20">
              <span className="material-symbols-outlined text-white text-[18px] font-bold">memory</span>
            </div>
            <h1 className="text-white text-xl font-black tracking-tighter uppercase">MEMCP</h1>
          </div>
          <p className="text-text-secondary text-[10px] font-mono uppercase tracking-widest opacity-50">v1.1.0 Feature-Complete</p>
        </div>

        <div className="px-2">
          <label className="text-[9px] font-black text-text-secondary uppercase tracking-widest mb-1.5 block">Project Context</label>
          <div className="relative">
            <select
              className="w-full bg-surface-dark border border-[#233f48] rounded-lg text-xs font-bold text-white px-3 py-2 appearance-none focus:ring-1 focus:ring-primary outline-none cursor-pointer"
              value={currentContext}
              onChange={(e) => setContext(e.target.value)}
            >
              {contexts.map(ctx => <option key={ctx} value={ctx}>{ctx.toUpperCase()}</option>)}
            </select>
            <span className="material-symbols-outlined absolute right-2 top-1/2 -translate-y-1/2 text-text-secondary text-sm pointer-events-none">expand_more</span>
          </div>
        </div>

        <nav className="flex flex-col gap-1">
          <SidebarLink href="/" icon="dashboard" label="Dashboard" />
          <SidebarLink href="/search" icon="search" label="Memory Search" />
          <SidebarLink href="/graph" icon="hub" label="Entity Explorer" />
          <hr className="border-[#233f48] my-1 mx-2" />
          <SidebarLink href="/episodes" icon="history" label="Episodes (Logs)" />
          <SidebarLink href="/procedures" icon="terminal" label="Procedures" />
          <hr className="border-[#233f48] my-1 mx-2" />
          <SidebarLink href="/ingest" icon="cloud_upload" label="Ingest Files" />
          <SidebarLink href="/new" icon="add_circle" label="New Node" />
          <SidebarLink href="/maintenance" icon="build" label="Maintenance" />
        </nav>
      </div>
      <div className="p-4 rounded-xl bg-surface-dark border border-[#233f48] flex items-center justify-between">
        <div>
          <p className="text-[10px] font-bold text-text-secondary uppercase tracking-widest mb-1">Health Index</p>
          <p className="text-primary font-mono font-black text-lg">98%</p>
        </div>
        <div className="size-8 rounded-full border-4 border-[#233f48] border-t-primary rotate-45 shadow-[0_0_10px_rgba(19,182,236,0.3)]"></div>
      </div>
    </aside>
  );
}
