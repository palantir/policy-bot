(function() {
  const hidden = [];

  function toggleFilter(target) {
    if (target.checked) {
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

  document.addEventListener('DOMContentLoaded', () => {
    const toggle = document.getElementById('filter-toggle');
    if (toggle) {
      toggleFilter(toggle);
      toggle.addEventListener('change', (event) => toggleFilter(event.target));
    }
  });
})();
