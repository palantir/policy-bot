(function() {
  const toggle = document.getElementById('theme-toggle');
  if (toggle) {
    toggle.checked = document.documentElement.classList.contains('dark');
    toggle.addEventListener('change', ({ target }) => {
      document.documentElement.classList.toggle('dark', target.checked);
      localStorage.setItem('theme', target.checked ? 'dark' : 'light');
    });
  }
})();
