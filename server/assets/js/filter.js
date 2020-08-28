(function() {
  const hidden = [];

  function toggleFilter(evt) {
    const enabled = evt.target.checked;
    if (enabled) {
      document
        .querySelectorAll('[data-status="skipped"]')
        .forEach(elem => {
          hidden.push({
            element: elem,
            parent: elem.parentElement,
            before: elem.nextSibling,
          });
          elem.remove();
        });
    } else {
      while (hidden.length) {
        const { element, parent, before } = hidden.pop();
        parent.insertBefore(element, before);
      }
    }
  }

  const toggle = document.getElementById('filter-toggle');
  if (toggle) {
    toggle.addEventListener('change', toggleFilter);
  }
})();
