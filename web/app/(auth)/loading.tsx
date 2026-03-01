export default function AuthLoading() {
  return (
    <div className="animate-pulse space-y-4">
      <div className="mx-auto h-8 w-40 rounded-lg bg-slate-200 dark:bg-slate-800" />
      <div className="mx-auto h-4 w-56 rounded bg-slate-100 dark:bg-slate-800/60" />
      <div className="mt-4 h-12 rounded-xl bg-slate-100 dark:bg-slate-800/60" />
    </div>
  );
}
