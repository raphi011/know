export default function MainLoading() {
  return (
    <div className="animate-pulse space-y-4 p-6">
      <div className="h-8 w-48 rounded-lg bg-slate-200 dark:bg-slate-800" />
      <div className="h-4 w-64 rounded bg-slate-100 dark:bg-slate-800/60" />
      <div className="mt-6 h-40 rounded-xl bg-slate-100 dark:bg-slate-800/60" />
    </div>
  );
}
