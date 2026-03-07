// Theme management — prevents FOUC by reading cookie before paint
(function() {
  const theme = document.cookie.match(/kh_theme=(\w+)/)?.[1] || 'system';
  const isDark = theme === 'dark' || (theme === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.classList.toggle('dark', isDark);
})();

// Cmd+K / Ctrl+K shortcut for search
document.addEventListener('keydown', function(e) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault();
    const dialog = document.getElementById('search-dialog');
    if (dialog) {
      dialog.showModal();
      dialog.querySelector('input')?.focus();
    }
  }
});
