/* Shared language preference and native disclosure navigation for both pages. */
(() => {
  const button = document.getElementById('language-toggle');
  let language = 'cn';
  function setLanguage(next) {
    language = next === 'en' ? 'en' : 'cn';
    document.documentElement.lang = language === 'en' ? 'en' : 'zh-CN';
    for (const element of document.querySelectorAll('[data-cn][data-en]')) {
      element.textContent = element.getAttribute(`data-${language}`);
    }
    for (const attribute of ['aria-label', 'alt']) {
      for (const element of document.querySelectorAll(`[data-cn-${attribute}][data-en-${attribute}]`)) {
        element.setAttribute(attribute, element.getAttribute(`data-${language}-${attribute}`));
      }
    }
    button.textContent = language === 'cn' ? 'English' : '中文';
    button.setAttribute('aria-label', language === 'cn' ? 'Switch language to English' : '切换为中文');
    try { localStorage.setItem('modmux-language', language); } catch (_) { /* Storage is optional. */ }
  }
  try { language = localStorage.getItem('modmux-language') || 'cn'; } catch (_) { /* Keep the readable default. */ }
  const requestedLanguage = new URLSearchParams(location.search).get('lang');
  if (['en', 'cn'].includes(requestedLanguage)) language = requestedLanguage;
  setLanguage(language);
  button.addEventListener('click', () => setLanguage(language === 'cn' ? 'en' : 'cn'));
  function openTarget(hash) {
    const target = document.getElementById(hash.replace(/^#/, ''));
    if (!target) return;
    const details = target.closest('details');
    if (details) details.open = true;
  }
  document.addEventListener('click', event => {
    const link = event.target.closest('a[data-open-details]');
    if (link) openTarget(link.hash);
  });
  window.addEventListener('hashchange', () => openTarget(location.hash));
  openTarget(location.hash);
})();
