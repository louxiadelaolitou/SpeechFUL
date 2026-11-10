import './demo.js';

document.documentElement.classList.add('js');

const toggle = document.querySelector('.menu-toggle');
const navigation = document.querySelector('#navigation');
toggle.hidden = false;
function closeNavigation() {
  toggle.setAttribute('aria-expanded', 'false');
  toggle.querySelector('.sr-only').textContent = '打开导航';
  navigation.classList.remove('is-open');
}
toggle.addEventListener('click', () => {
  const open = toggle.getAttribute('aria-expanded') !== 'true';
  toggle.setAttribute('aria-expanded', String(open));
  toggle.querySelector('.sr-only').textContent = open ? '关闭导航' : '打开导航';
  navigation.classList.toggle('is-open', open);
});
navigation.addEventListener('click', (event) => {
  if (event.target.closest('a')) closeNavigation();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && navigation.classList.contains('is-open')) {
    closeNavigation();
    toggle.focus();
  }
});
document.addEventListener('click', (event) => {
  if (!event.target.closest('.nav-shell')) closeNavigation();
});
matchMedia('(min-width: 601px)').addEventListener('change', closeNavigation);

const mascot = document.querySelector('.mascot-trigger');
const hero = document.querySelector('.hero');
const reduceMotion = matchMedia('(prefers-reduced-motion: reduce)');

// Observe stable sections rather than their animated children. A clipped title
// must not repeatedly trigger its own intersection and restart its animation.
// Content stays visible without JavaScript, with reduced motion, or without IO.
if ('IntersectionObserver' in window && !reduceMotion.matches) {
  const revealTargets = new Map([
    [document.querySelector('.about-section'), '.about-section > h2, .app-demo'],
    [document.querySelector('.download-section'), '.download-heading, .download-options .platform'],
    [document.querySelector('.contact-section'), '.contact-heading, .contact-details'],
  ]);
  const revealObserver = new IntersectionObserver((entries) => {
    for (const entry of entries) {
      entry.target.querySelectorAll(revealTargets.get(entry.target)).forEach((element) => {
        if (entry.isIntersecting) {
          if (element.matches(':focus-within')) element.classList.add('reveal-settled');
          element.classList.add('scroll-enter');
        } else {
          element.classList.remove('scroll-enter', 'reveal-settled');
        }
      });
    }
  }, { rootMargin: '0px 0px -6% 0px', threshold: 0.12 });
  revealTargets.forEach((selector, section) => {
    if (!section) return;
    section.querySelectorAll(selector).forEach((element) => {
      // Once focused, finish the entrance until this section leaves view.
      element.addEventListener('focusin', () => element.classList.add('reveal-settled'));
    });
    revealObserver.observe(section);
  });
}
let animationEnd;
function playLogo() {
  if (reduceMotion.matches || mascot.classList.contains('playing')) return;
  clearTimeout(animationEnd);
  mascot.classList.add('playing');
  hero.classList.add('is-polishing');
  animationEnd = setTimeout(() => {
    mascot.classList.remove('playing');
    hero.classList.remove('is-polishing');
  }, 2650);
}
mascot.addEventListener('click', playLogo);
mascot.addEventListener('pointerenter', (event) => {
  if (event.pointerType === 'mouse') playLogo();
});
mascot.addEventListener('focus', playLogo);
reduceMotion.addEventListener('change', () => {
  if (reduceMotion.matches) {
    clearTimeout(animationEnd);
    mascot.classList.remove('playing');
    hero.classList.remove('is-polishing');
  }
});
// One greeting on arrival. No permanent animation loop or scroll handlers.
setTimeout(playLogo, 500);
