(function() {
  const toggle = document.getElementById('filter-toggle');
  if (toggle) {
    toggle.addEventListener('change', ({ target }) => {
      document
        .querySelectorAll('.tree[data-hide-status]')
        .forEach(elem => elem.setAttribute('data-hide-status', target.checked ? 'skipped' : ''));
    });
  }
})();
