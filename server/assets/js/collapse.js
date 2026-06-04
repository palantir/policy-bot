(function () {
  const controls = document.querySelectorAll('[data-rule-collapse]');
  if (controls.length === 0) return;

  const cards = () => Array.from(document.querySelectorAll('.pb-card'));

  controls.forEach((control) => {
    control.addEventListener('click', () => {
      const open = control.getAttribute('data-rule-collapse') === 'expand';
      cards().forEach((card) => {
        card.open = open;
      });
    });
  });
})();
