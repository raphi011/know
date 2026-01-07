'use client';

import React, { createContext, useContext, useState, useCallback, ReactNode } from 'react';

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

export function AppProvider({ children }: { children: ReactNode }) {
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
              toast.type === 'success' ? 'bg-surface-dark border-green-500/30 text-green-400'
              : toast.type === 'error' ? 'bg-surface-dark border-red-500/30 text-red-400'
              : 'bg-surface-dark border-orange-500/30 text-orange-400'
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
}
