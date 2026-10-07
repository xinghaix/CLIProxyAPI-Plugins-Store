import { describe, expect, it, vi } from 'vitest';
import {
  FOCUSABLE_SELECTOR,
  cycleTabFocus,
} from './focusTrap.js';

function stubEl(id) {
  return {
    id,
    focus: vi.fn(),
  };
}

describe('focusTrap', () => {
  it('exports a usable focusable selector covering buttons and links', () => {
    expect(FOCUSABLE_SELECTOR).toContain('button:not([disabled])');
    expect(FOCUSABLE_SELECTOR).toContain('a[href]');
    expect(FOCUSABLE_SELECTOR).toContain('[tabindex]:not([tabindex="-1"])');
  });

  it('Tab on last cycles to first', () => {
    const a = stubEl('a');
    const b = stubEl('b');
    const c = stubEl('c');
    const event = { key: 'Tab', shiftKey: false, preventDefault: vi.fn() };
    expect(cycleTabFocus(event, [a, b, c], c)).toBe(true);
    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(a.focus).toHaveBeenCalledOnce();
    expect(b.focus).not.toHaveBeenCalled();
  });

  it('Shift+Tab on first cycles to last', () => {
    const a = stubEl('a');
    const b = stubEl('b');
    const c = stubEl('c');
    const event = { key: 'Tab', shiftKey: true, preventDefault: vi.fn() };
    expect(cycleTabFocus(event, [a, b, c], a)).toBe(true);
    expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(c.focus).toHaveBeenCalledOnce();
  });

  it('Tab in the middle does not intercept', () => {
    const a = stubEl('a');
    const b = stubEl('b');
    const c = stubEl('c');
    const event = { key: 'Tab', shiftKey: false, preventDefault: vi.fn() };
    expect(cycleTabFocus(event, [a, b, c], b)).toBe(false);
    expect(event.preventDefault).not.toHaveBeenCalled();
    expect(a.focus).not.toHaveBeenCalled();
    expect(c.focus).not.toHaveBeenCalled();
  });

  it('Tab when focus is outside list jumps to first', () => {
    const a = stubEl('a');
    const b = stubEl('b');
    const outside = stubEl('out');
    const event = { key: 'Tab', shiftKey: false, preventDefault: vi.fn() };
    expect(cycleTabFocus(event, [a, b], outside)).toBe(true);
    expect(a.focus).toHaveBeenCalledOnce();
  });

  it('Shift+Tab when focus is outside list jumps to last', () => {
    const a = stubEl('a');
    const b = stubEl('b');
    const outside = stubEl('out');
    const event = { key: 'Tab', shiftKey: true, preventDefault: vi.fn() };
    expect(cycleTabFocus(event, [a, b], outside)).toBe(true);
    expect(b.focus).toHaveBeenCalledOnce();
  });

  it('Tab with empty focusables still preventsDefault', () => {
    const event = { key: 'Tab', shiftKey: false, preventDefault: vi.fn() };
    expect(cycleTabFocus(event, [], null)).toBe(true);
    expect(event.preventDefault).toHaveBeenCalledOnce();
  });

  it('ignores non-Tab keys', () => {
    const event = { key: 'Escape', preventDefault: vi.fn() };
    expect(cycleTabFocus(event, [stubEl('a')], null)).toBe(false);
    expect(event.preventDefault).not.toHaveBeenCalled();
  });
});
