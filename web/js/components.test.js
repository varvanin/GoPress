import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';

const bundle = readFileSync(new URL('../../internal/httpui/assets/components.bundle.js', import.meta.url), 'utf8');

// Check the real bundled hooks at startup without pretending to render a DOM.
function runtime() {
  const listeners = new Map();
  class Element {
    constructor(editable = false) { this.editable = editable; }
    closest() { return this.editable ? this : null; }
  }
  const attributes = new Map([['data-state', 'expanded'], ['data-templ-collapsible', 'offcanvas'], ['data-templ-sidebar-id', 'gopress-sidebar']]);
  const wrapper = { getAttribute: (name) => attributes.get(name), setAttribute: (name, value) => attributes.set(name, value), querySelectorAll: () => [] };
  const document = {
    readyState: 'loading', documentElement: {},
    addEventListener(name, fn) { if (!listeners.has(name)) listeners.set(name, []); listeners.get(name).push(fn); },
    querySelector: (selector) => selector.includes('mobile-portal') ? null : wrapper,
    querySelectorAll: () => [],
  };
  const window = { addEventListener() {}, matchMedia: () => ({ matches: false }) };
  runInNewContext(bundle, { document, window, Element, navigator: {platform: "MacIntel", userAgent: "Test browser"}, MutationObserver: class { observe() {} }, setTimeout, clearTimeout });
  return { window, attributes, key(editable, prevented = false) {
    const event = { ctrlKey: true, key: 'b', target: new Element(editable), defaultPrevented: prevented, preventDefault() { this.defaultPrevented = true; } };
    for (const fn of listeners.get('keydown') ?? []) fn(event);
    return event;
  } };
}

test('shadcn runtime supplies sidebar and select dependencies', () => {
  const { window } = runtime();
  for (const name of ['lifecycle', 'tabbable', 'portal', 'transition', 'dismiss', 'focusManager', 'scrollLock', 'click', 'dialog', 'sidebar', 'direction', 'composite', 'listNavigation', 'typeahead', 'anchorPositioning', 'internalBackdrop', 'select']) assert.ok(window.templ[name], name);
  assert.equal(typeof window.templ.markOthers, 'function');
  assert.equal(typeof window.FloatingUIDOM.computePosition,'function');
  assert.equal(typeof window.FloatingUICore.computePosition,'function');
});

test('sidebar shortcut never steals bold or field shortcuts from the editor', () => {
  const ui = runtime();
  ui.key(true);
  assert.equal(ui.attributes.get('data-state'), 'expanded');
  ui.key(false, true);
  assert.equal(ui.attributes.get('data-state'), 'expanded');
  const event = ui.key(false);
  assert.equal(ui.attributes.get('data-state'), 'collapsed');
  assert.equal(event.defaultPrevented, true);
});
