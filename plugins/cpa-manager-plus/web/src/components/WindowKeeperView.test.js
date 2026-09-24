import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createRenderer, nextTick, ssrContextKey } from 'vue';
import { createI18n } from 'vue-i18n';
import WindowKeeperView from './WindowKeeperView.vue';
import zhCN from '../i18n/messages/zh-CN.js';

const renderer = createRenderer({
  createComment: () => ({}),
  insert: () => {},
  remove: () => {},
  parentNode: () => null,
  nextSibling: () => null,
});

describe('WindowKeeperView controller', () => {
  let app;
  let state;
  let proxyCallMock;

  beforeEach(() => {
    proxyCallMock = vi.fn().mockImplementation((payloadOrMethod, maybePath) => {
      const path = typeof payloadOrMethod === 'object' ? payloadOrMethod.path : maybePath;
      if (path && path.endsWith('/settings')) {
        return Promise.resolve({
          settings: { enabled: true, model: 'gpt-5.4', effort: 'none', poll_seconds: 20 },
          management_key_set: true,
        });
      }
      if (path && path.endsWith('/accounts')) {
        return Promise.resolve({
          accounts: [
            {
              auth_id: 'ada-1',
              email: 'ada@example.com',
              plan_type: 'plus',
              windows: [
                {
                  limit_id: 'codex',
                  slot: 'primary',
                  kind: 'five_hour',
                  period_seconds: 18000,
                  used_percent: 45,
                  gating: true,
                  phase: 'clear',
                },
              ],
            },
          ],
        });
      }
      if (path && path.endsWith('/attempts')) {
        return Promise.resolve({ attempts: [] });
      }
      return Promise.resolve({});
    });

    app = renderer.createApp({ ...WindowKeeperView, render: () => null }, { ready: true, proxyCall: proxyCallMock });
    app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }));
    app.provide(ssrContextKey, {});
    const vm = app.mount({});
    state = vm.$.setupState;
  });

  afterEach(() => {
    app?.unmount();
    vi.unstubAllGlobals();
  });

  it('loads settings and accounts on mount', async () => {
    await nextTick();
    await new Promise(r => setTimeout(r, 50));

    expect(proxyCallMock).toHaveBeenCalledWith(expect.objectContaining({ method: 'GET', path: '/v0/management/window-keeper/settings' }));
    expect(proxyCallMock).toHaveBeenCalledWith(expect.objectContaining({ method: 'GET', path: '/v0/management/window-keeper/accounts' }));
    expect(state.accounts.length).toBe(1);
    expect(state.accounts[0].email).toBe('ada@example.com');
    expect(state.settings.enabled).toBe(true);
    expect(state.hasManagementKey).toBe(true);
  });

  it('toggles global switch', async () => {
    await nextTick();
    await new Promise(r => setTimeout(r, 50));

    await state.toggleGlobalSwitch();
    expect(proxyCallMock).toHaveBeenCalledWith(expect.objectContaining({ method: 'PUT', path: '/v0/management/window-keeper/settings', body: expect.objectContaining({ enabled: false }) }));
    expect(state.settings.enabled).toBe(false);
  });
});
