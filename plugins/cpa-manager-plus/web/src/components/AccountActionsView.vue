<template>
  <section class="monitoring-page">
    <div class="card filter-card" style="flex-direction:row;gap:10px;flex-wrap:wrap">
      <select v-model="filter" class="control compact" @change="loadCandidates">
        <option value="pending">{{ t('accountActions.filter.pending') }}</option>
        <option value="all">{{ t('accountActions.filter.all') }}</option>
        <option value="ignored">{{ t('accountActions.filter.ignored') }}</option>
        <option value="resolved">{{ t('accountActions.filter.resolved') }}</option>
        <option value="deleted">{{ t('accountActions.filter.deleted') }}</option>
      </select>
      <input v-model.trim="search" class="control wide" :placeholder="t('accountActions.searchPlaceholder')" @keyup.enter="loadCandidates" />
      <button class="btn primary" @click="loadCandidates" :disabled="loading">{{ loading ? t('common.loading') : t('common.refresh') }}</button>
    </div>

    <section v-if="error" class="notice error">{{ error }}</section>

    <DataCard :title="t('accountActions.title')" :subtitle="cardSubtitle">
      <div class="table-wrap monitor-table">
        <table>
          <thead>
            <tr>
              <th>{{ t('accountActions.columns.account') }}</th>
              <th>{{ t('accountActions.columns.provider') }}</th>
              <th>{{ t('accountActions.columns.authFile') }}</th>
              <th>{{ t('accountActions.columns.actionType') }}</th>
              <th>{{ t('accountActions.columns.errorKind') }}</th>
              <th>{{ t('accountActions.columns.errorCode') }}</th>
              <th>{{ t('accountActions.columns.status') }}</th>
              <th>{{ t('accountActions.columns.triggeredAt') }}</th>
              <th>{{ t('accountActions.columns.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in visibleItems" :key="item.id" :class="{selected: actingId === item.id}">
              <td>
                <div>{{ item.account_snapshot || item.accountSnapshot || EMPTY_VALUE }}</div>
                <div class="muted small-text">{{ item.auth_label || item.authLabel || '' }}</div>
              </td>
              <td>{{ item.provider || EMPTY_VALUE }}</td>
              <td>{{ item.auth_file_name || item.authFileName || EMPTY_VALUE }}</td>
              <td><span :class="['status-badge', actionBadgeClass(item.action_type || item.actionType)]">{{ actionLabel(item.action_type || item.actionType) }}</span></td>
              <td>{{ item.error_kind || item.errorKind || item.header_error_kind || item.headerErrorKind || EMPTY_VALUE }}</td>
              <td>{{ item.error_code || item.errorCode || item.header_error_code || item.headerErrorCode || EMPTY_VALUE }}</td>
              <td><span :class="['status-badge', item.status === 'pending' ? 'bad' : 'good']">{{ statusLabel(item.status) }}</span></td>
              <td class="small-text">{{ formatDateTime(item.triggered_at_ms || item.triggeredAtMs) }}</td>
              <td>
                <div class="config-actions-bar" style="padding:0;gap:4px">
                  <button v-if="item.status === 'pending'" class="btn primary" size="xs" @click="act(item.id, 'enable')" :disabled="busy">{{ t('accountActions.actions.enable') }}</button>
                  <button v-if="item.status === 'pending'" class="btn" @click="act(item.id, 'ignore')" :disabled="busy">{{ t('accountActions.actions.ignore') }}</button>
                  <button v-if="item.status === 'pending'" class="btn danger" @click="act(item.id, 'delete')" :disabled="busy">{{ t('accountActions.actions.delete') }}</button>
                  <button v-if="item.status === 'pending'" class="btn" @click="act(item.id, 'resolve')" :disabled="busy">{{ t('accountActions.actions.resolve') }}</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </DataCard>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import DataCard from './DataCard.vue';
import { accountActionPath } from '../utils/data.js';
import { EMPTY_VALUE, formatDateTime } from '../utils/localeFormat.js';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const { t } = useI18n();

const items = ref([]);
const loading = ref(false);
const busy = ref(false);
const error = ref('');
const filter = ref('pending');
const search = ref('');
const actingId = ref(null);

const filterLabel = computed(() => {
  const map = {
    pending: 'accountActions.filter.pending',
    all: 'accountActions.filter.all',
    ignored: 'accountActions.filter.ignored',
    resolved: 'accountActions.filter.resolved',
    deleted: 'accountActions.filter.deleted',
  };
  return t(map[filter.value] || 'accountActions.filter.all');
});

const cardSubtitle = computed(() => t('accountActions.subtitle', {
  filter: filterLabel.value,
  count: visibleItems.value.length,
}));

const visibleItems = computed(() => {
  const term = search.value.trim().toLowerCase();
  if (!term) return items.value;
  return items.value.filter(item => {
    const fields = [
      item.account_snapshot, item.accountSnapshot,
      item.auth_label, item.authLabel,
      item.provider, item.auth_file_name, item.authFileName,
      item.error_kind, item.errorKind, item.header_error_kind, item.headerErrorKind,
      item.error_code, item.errorCode, item.header_error_code, item.headerErrorCode,
    ];
    return fields.some(f => f && String(f).toLowerCase().includes(term));
  });
});

onMounted(() => { if (props.ready) loadCandidates(); });

async function loadCandidates() {
  if (!props.ready) return;
  loading.value = true;
  error.value = '';
  try {
    const params = { status: filter.value === 'all' ? '' : filter.value, limit: 200 };
    const qs = params.status ? `status=${encodeURIComponent(params.status)}&limit=${params.limit}` : `limit=${params.limit}`;
    const resp = await props.proxyCall({
      method: 'GET',
      path: '/v0/management/account-action-candidates',
      query: qs,
    });
    items.value = resp?.items || [];
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    loading.value = false;
  }
}

async function act(id, action) {
  busy.value = true;
  actingId.value = id;
  try {
    await props.proxyCall({
      method: action === 'delete' ? 'DELETE' : 'POST',
      path: accountActionPath(id, action),
    });
    await loadCandidates();
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    busy.value = false;
    actingId.value = null;
  }
}

function actionLabel(a) {
  const map = {
    delete: 'accountActions.actionType.delete',
    reauth: 'accountActions.actionType.reauth',
    enable: 'accountActions.actionType.enable',
    keep: 'accountActions.actionType.keep',
    review: 'accountActions.actionType.review',
  };
  if (map[a]) return t(map[a]);
  return a || EMPTY_VALUE;
}

function actionBadgeClass(a) {
  return a === 'delete' ? 'bad' : a === 'reauth' ? 'warn' : 'good';
}

function statusLabel(s) {
  const map = {
    pending: 'accountActions.status.pending',
    ignored: 'accountActions.status.ignored',
    resolved: 'accountActions.status.resolved',
    deleted: 'accountActions.status.deleted',
  };
  if (map[s]) return t(map[s]);
  return s || EMPTY_VALUE;
}

defineExpose({ refresh: loadCandidates });
</script>
