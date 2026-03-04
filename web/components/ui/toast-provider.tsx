"use client";

import {
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Toast } from "./toast";

type ToastVariant = "success" | "error" | "info";

interface ToastItem {
  id: string;
  variant: ToastVariant;
  title: string;
  description?: string;
  duration?: number;
}

interface ToastInput {
  variant: ToastVariant;
  title: string;
  description?: string;
  duration?: number;
}

interface ToastContextValue {
  toasts: ToastItem[];
  toast: (input: ToastInput) => void;
  dismiss: (id: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

let nextId = 0;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const timers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  const dismiss = useCallback((id: string) => {
    const timer = timers.current.get(id);
    if (timer) {
      clearTimeout(timer);
      timers.current.delete(id);
    }
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const toast = useCallback(
    (input: ToastInput) => {
      const id = String(++nextId);
      const duration = input.duration ?? 4000;
      setToasts((prev) => [...prev, { ...input, id }]);
      const timer = setTimeout(() => dismiss(id), duration);
      timers.current.set(id, timer);
    },
    [dismiss],
  );

  return (
    <ToastContext value={{ toasts, toast, dismiss }}>
      {children}
      <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
        {toasts.map((t) => (
          <Toast
            key={t.id}
            variant={t.variant}
            title={t.title}
            description={t.description}
            open={true}
            onClose={() => dismiss(t.id)}
          />
        ))}
      </div>
    </ToastContext>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}
