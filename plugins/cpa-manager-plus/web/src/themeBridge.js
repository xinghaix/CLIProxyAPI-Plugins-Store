/**
 * Theme bridge: copies CSS custom properties and data-theme attribute from
 * the CPA host (parent window) into the plugin iframe's documentElement.
 *
 * CPA sets theme variables on its own :root, but the iframe is a separate
 * browsing context so they don't cascade. This script bridges that gap.
 */
const THEME_VARS = [
  '--bg-primary', '--bg-secondary', '--bg-tertiary', '--bg-hover',
  '--bg-quinary', '--bg-error-light', '--floating-surface', '--floating-border',
  '--floating-shadow',
  '--text-primary', '--text-secondary', '--text-tertiary', '--text-quaternary',
  '--text-muted',
  '--border-color', '--border-secondary', '--border-primary', '--border-hover',
  '--primary-color', '--primary-hover', '--primary-active', '--primary-contrast',
  '--primary-8', '--primary-10', '--primary-30',
  '--success-color', '--warning-color', '--error-color', '--danger-color',
  '--info-color', '--quota-medium-color',
  '--warning-bg', '--warning-border', '--warning-text',
  '--success-badge-bg', '--success-badge-text', '--success-badge-border',
  '--failure-badge-bg', '--failure-badge-text', '--failure-badge-border',
  '--count-badge-bg', '--count-badge-text',
  '--shadow', '--shadow-lg',
  '--radius-md', '--accent-tertiary',
  '--amber-color', '--amber-text', '--amber-10', '--amber-30',
  '--destructive-color', '--destructive-10', '--destructive-30',
  '--muted-bg', '--muted-foreground', '--accent-bg',
  '--glass-blur', '--glass-backdrop-filter', '--glass-filter',
  '--glass-bg', '--glass-bg-secondary', '--glass-border',
];

function syncThemeFromParent() {
  try {
    const parentRoot = window.parent.document.documentElement;
    const computed = window.parent.getComputedStyle(parentRoot);
    const ourRoot = document.documentElement;
    for (const varName of THEME_VARS) {
      const value = computed.getPropertyValue(varName).trim();
      if (value) {
        ourRoot.style.setProperty(varName, value);
      }
    }
    // Sync data-theme attribute
    const dataTheme = parentRoot.getAttribute('data-theme');
    if (dataTheme) {
      ourRoot.setAttribute('data-theme', dataTheme);
    } else {
      ourRoot.removeAttribute('data-theme');
    }
  } catch {
    // Cross-origin parent: cannot access, fallbacks in :root will apply
  }
}

function initThemeBridge() {
  syncThemeFromParent();

  // Watch for theme changes in parent (data-theme attribute mutations)
  try {
    const parentRoot = window.parent.document.documentElement;
    const observer = new MutationObserver(() => syncThemeFromParent());
    observer.observe(parentRoot, {
      attributes: true,
      attributeFilter: ['data-theme', 'style', 'class'],
    });
    // Also re-sync on our own load and on window resize (some vars are responsive)
    window.addEventListener('load', syncThemeFromParent);
  } catch {
    // Cross-origin: fallback only
  }
}

export { initThemeBridge, syncThemeFromParent };
