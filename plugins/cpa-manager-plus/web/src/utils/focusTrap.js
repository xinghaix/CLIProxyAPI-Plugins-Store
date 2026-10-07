/**
 * Lightweight Tab focus-trap helpers (no a11y library).
 * Used by CredentialQuotaDrawer when aria-modal is open.
 */

/** @type {string} */
export const FOCUSABLE_SELECTOR = [
  'a[href]',
  'area[href]',
  'button:not([disabled])',
  'input:not([disabled]):not([type="hidden"])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ');

/**
 * @param {Element | null | undefined} el
 * @returns {boolean}
 */
export function isElementFocusable(el) {
  if (!el || !(el instanceof Element)) return false;
  if (el.hasAttribute('disabled') || el.getAttribute('aria-hidden') === 'true') return false;
  if (typeof el.checkVisibility === 'function') {
    try {
      if (!el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) return false;
    } catch {
      /* older engines */
    }
  } else {
    const style = el.ownerDocument?.defaultView?.getComputedStyle?.(el);
    if (style && (style.display === 'none' || style.visibility === 'hidden')) return false;
  }
  if (el.closest?.('[hidden], [aria-hidden="true"]')) return false;
  return true;
}

/**
 * @param {ParentNode | null | undefined} container
 * @returns {HTMLElement[]}
 */
export function getFocusableElements(container) {
  if (!container || typeof container.querySelectorAll !== 'function') return [];
  return Array.from(container.querySelectorAll(FOCUSABLE_SELECTOR))
    .filter((el) => isElementFocusable(el) && el instanceof HTMLElement);
}

/**
 * Cycle Tab / Shift+Tab within focusables. Pure enough for unit tests.
 * @param {{ key: string, shiftKey?: boolean, preventDefault?: () => void }} event
 * @param {Array<{ focus: () => void }>} focusables
 * @param {{ focus?: () => void } | null | undefined} activeElement
 * @returns {boolean} true if the event was handled (preventDefault called)
 */
export function cycleTabFocus(event, focusables, activeElement) {
  if (!event || event.key !== 'Tab') return false;
  const list = Array.isArray(focusables) ? focusables : [];
  if (!list.length) {
    event.preventDefault?.();
    return true;
  }
  const first = list[0];
  const last = list[list.length - 1];
  const active = activeElement ?? null;
  const inList = active != null && list.includes(active);

  if (event.shiftKey) {
    if (!inList || active === first) {
      event.preventDefault?.();
      last.focus?.();
      return true;
    }
  } else if (!inList || active === last) {
    event.preventDefault?.();
    first.focus?.();
    return true;
  }
  return false;
}

/**
 * Handle Tab keydown against a live container (drawer panel).
 * @param {KeyboardEvent} event
 * @param {ParentNode | null | undefined} container
 * @returns {boolean}
 */
export function trapTabKeydown(event, container) {
  if (!event || event.key !== 'Tab' || !container) return false;
  const focusables = getFocusableElements(container);
  const doc = container.ownerDocument || (typeof document !== 'undefined' ? document : null);
  return cycleTabFocus(event, focusables, doc?.activeElement);
}

/**
 * Move initial focus into the trap. Prefers `preferredSelector` when present
 * and focusable (Close button), else first focusable in container.
 * @param {ParentNode | null | undefined} container
 * @param {string} [preferredSelector='.cred-drawer-close']
 * @returns {HTMLElement | null}
 */
export function focusInitialIn(container, preferredSelector = '.cred-drawer-close') {
  if (!container) return null;
  if (preferredSelector && typeof container.querySelector === 'function') {
    const preferred = container.querySelector(preferredSelector);
    if (preferred instanceof HTMLElement && isElementFocusable(preferred)) {
      preferred.focus();
      return preferred;
    }
  }
  const first = getFocusableElements(container)[0] || null;
  first?.focus?.();
  return first;
}
