// Theme management — prevents FOUC by reading cookie before paint
(function() {
  const theme = document.cookie.match(/kh_theme=(\w+)/)?.[1] || 'system';
  const isDark = theme === 'dark' || (theme === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.classList.toggle('dark', isDark);
})();
