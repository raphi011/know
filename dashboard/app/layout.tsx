import type { Metadata } from 'next';
import './globals.css';
import { AppProvider } from '@/components/AppProvider';
import { Sidebar } from '@/components/Sidebar';

export const metadata: Metadata = {
  title: 'memcp Dashboard',
  description: 'Memory Control Plane - Knowledge Graph Dashboard',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <head>
        <link
          rel="stylesheet"
          href="https://fonts.googleapis.com/css2?family=Material+Symbols+Outlined:opsz,wght,FILL,GRAD@20..48,100..700,0..1,-50..200"
        />
      </head>
      <body className="antialiased">
        <AppProvider>
          <div className="flex h-screen bg-background-dark text-slate-200 overflow-hidden font-sans selection:bg-primary/30">
            <Sidebar />
            <main className="flex-1 flex flex-col relative overflow-hidden bg-background-dark">
              {children}
            </main>
          </div>
        </AppProvider>
      </body>
    </html>
  );
}
