
import React, { createContext, useContext, useState, useCallback, ReactNode, useEffect } from 'react';
import { HashRouter as Router, Routes, Route, Link, useLocation } from 'react-router-dom';
import Overview from './screens/Overview';
import Search from './screens/Search';
import GraphView from './screens/GraphView';
import EntityDetail from './screens/EntityDetail';
import AddMemory from './screens/AddMemory';
import Ingest from './screens/Ingest';
import Maintenance from './screens/Maintenance';
import Episodes from './screens/Episodes';
import EpisodeDetail from './screens/EpisodeDetail';
import Procedures from './screens/Procedures';
import ProcedureEditor from './screens/ProcedureEditor';
import { backend } from './backend';

type ToastType = 'success' | 'error' | 'warning';

interface Toast {
  id: number;
  message: string;
  type: ToastType;
}

interface AppContextType {
  showToast: (message: string, type?: ToastType) => void;
  currentContext: string;
  setContext: (ctx: string) => void;
}

const AppContext = createContext<AppContextType | undefined>(undefined);

export const useApp = () => {
  const context = useContext(AppContext);
  if (!context) throw new Error('useApp must be used within an AppProvider');
  return context;
};

const AppProvider = ({ children }: { children?: ReactNode }) => {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [currentContext, setContext] = useState('all');

  const showToast = useCallback((message: string, type: ToastType = 'success') => {
    const id = Date.now();
    setToasts((prev) => [...prev, { id, message, type }]);
    setTimeout(() => {
      setToasts((prev) => prev.filter((t) => t.id !== id));
    }, 3000);
  }, []);

  return (
    <AppContext.Provider value={{ showToast, currentContext, setContext }}>
      {children}
      <div className="fixed bottom-8 left-1/2 -translate-x-1/2 z-[100] flex flex-col gap-2 items-center pointer-events-none w-full max-w-md px-4">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={`flex items-center gap-3 px-5 py-3 rounded-xl shadow-2xl border pointer-events-auto animate-page-in transition-all duration-300 w-full ${
              toast.type === 'success' ? 'bg-[#1a2c35] border-green-500/30 text-green-400' 
              : toast.type === 'error' ? 'bg-[#1a2c35] border-red-500/30 text-red-400'
              : 'bg-[#1a2c35] border-orange-500/30 text-orange-400'
            }`}
          >
            <span className="material-symbols-outlined text-[20px]">
              {toast.type === 'success' ? 'check_circle' : toast.type === 'error' ? 'error' : 'warning'}
            </span>
            <p className="text-sm font-bold uppercase tracking-widest">{toast.message}</p>
          </div>
        ))}
      </div>
    </AppContext.Provider>
  );
};

const SidebarLink = ({ to, icon, label }: { to: string, icon: string, label: string }) => {
  const location = useLocation();
  const isActive = location.pathname === to || (to !== '/' && location.pathname.startsWith(to));
  return (
    <Link to={to} className={`flex items-center gap-3 px-3 py-2.5 rounded-lg group transition-all duration-200 border border-transparent ${
      isActive ? 'bg-primary/15 border-primary/20 shadow-inner' : 'hover:bg-surface-dark-highlight/40'
    }`}>
      <span className={`material-symbols-outlined transition-colors duration-200 ${isActive ? 'text-primary' : 'text-text-secondary group-hover:text-white'}`} style={{ fontVariationSettings: isActive ? "'FILL' 1" : "'FILL' 0" }}>{icon}</span>
      <p className={`font-medium text-sm transition-colors duration-200 ${isActive ? 'text-primary' : 'text-text-secondary group-hover:text-white'}`}>{label}</p>
    </Link>
  );
};

const Sidebar = () => {
  const { currentContext, setContext } = useApp();
  const [contexts, setContexts] = useState<string[]>([]);

  useEffect(() => {
    backend.listContexts().then(setContexts);
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
          <SidebarLink to="/" icon="dashboard" label="Dashboard" />
          <SidebarLink to="/search" icon="search" label="Memory Search" />
          <SidebarLink to="/graph" icon="hub" label="Entity Explorer" />
          <hr className="border-[#233f48] my-1 mx-2" />
          <SidebarLink to="/episodes" icon="history" label="Episodes (Logs)" />
          <SidebarLink to="/procedures" icon="terminal" label="Procedures" />
          <hr className="border-[#233f48] my-1 mx-2" />
          <SidebarLink to="/ingest" icon="cloud_upload" label="Ingest Files" />
          <SidebarLink to="/add" icon="add_circle" label="New Node" />
          <SidebarLink to="/maintenance" icon="build" label="Maintenance" />
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

const App = () => {
  return (
    <AppProvider>
      <Router>
        <div className="flex h-screen bg-background-dark text-slate-200 overflow-hidden font-sans selection:bg-primary/30">
          <Sidebar />
          <main className="flex-1 flex flex-col relative overflow-hidden bg-background-dark">
            <Routes>
              <Route path="/" element={<Overview />} />
              <Route path="/search" element={<Search />} />
              <Route path="/graph" element={<GraphView />} />
              <Route path="/episodes" element={<Episodes />} />
              <Route path="/episode/:id" element={<EpisodeDetail />} />
              <Route path="/procedures" element={<Procedures />} />
              <Route path="/procedure/new" element={<ProcedureEditor />} />
              <Route path="/procedure/edit/:id" element={<ProcedureEditor />} />
              <Route path="/entity/:id" element={<EntityDetail />} />
              <Route path="/add" element={<AddMemory />} />
              <Route path="/ingest" element={<Ingest />} />
              <Route path="/maintenance" element={<Maintenance />} />
            </Routes>
          </main>
        </div>
      </Router>
    </AppProvider>
  );
};

export default App;
