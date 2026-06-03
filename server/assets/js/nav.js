// Sidebar rule-index behaviour: highlight the rule currently in view and
// briefly flash a card when its sidebar entry is clicked.
(function () {
  const links = Array.from(document.querySelectorAll('.pb-rule-link'));
  if (links.length === 0) return;

  const byId = new Map(links.map((a) => [a.getAttribute('href').slice(1), a]));

  // Flash the target card on click for a clear "you are here" cue.
  links.forEach((a) => {
    a.addEventListener('click', () => {
      const card = document.getElementById(a.getAttribute('href').slice(1));
      if (!card) return;
      card.classList.add('pb-flash');
      setTimeout(() => card.classList.remove('pb-flash'), 900);
    });
  });

  // Highlight the sidebar entry for the topmost visible card.
  const cards = links
    .map((a) => document.getElementById(a.getAttribute('href').slice(1)))
    .filter(Boolean);

  const setActive = (id) => {
    links.forEach((a) => a.classList.toggle('is-active', a.getAttribute('href').slice(1) === id));
  };

  if ('IntersectionObserver' in window && cards.length) {
    const visible = new Set();
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((e) => {
          if (e.isIntersecting) visible.add(e.target.id);
          else visible.delete(e.target.id);
        });
        // pick the first card (in DOM order) that is currently visible
        const first = cards.find((c) => visible.has(c.id));
        if (first && byId.has(first.id)) setActive(first.id);
      },
      { rootMargin: '-180px 0px -55% 0px' }
    );
    cards.forEach((c) => observer.observe(c));
  }
})();
