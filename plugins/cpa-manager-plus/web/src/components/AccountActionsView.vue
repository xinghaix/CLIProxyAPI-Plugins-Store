<template>
  <section class="monitoring-page">
    <div class="card filter-card" style="flex-direction:row;gap:10px;flex-wrap:wrap">
      <select v-model="filter" class="control compact" @change="loadCandidates">
        <option value="pending">待处理</option>
        <option value="all">全部状态</option>
        <option value="ignored">已忽略</option>
        <option value="resolved">已解决</option>
        <option value="deleted">已删除</option>
      </select>
      <input v-model.trim="search" class="control wide" placeholder="搜索账号 / 文件 / 错误 / trace" @keyup.enter="loadCandidates" />
      <button class="btn primary" @click="loadCandidates" :disabled="loading">{{ loading ? '加载中…' : '刷新' }}</button>
    </div>

    <section v-if="error" class="notice error">{{ error }}</section>

    <DataCard title="认证异常" :subtitle="`${filter} · ${visibleItems.length} 条`">
      <div class="table-wrap monitor-table">
        <table>
          <thead>
            <tr>
              <th>账号</th>
              <th>Provider</th>
              <th>凭据文件</th>
              <th>操作类型</th>
              <th>错误类型</th>
              <th>错误码</th>
              <th>状态</th>
              <th>触发时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in visibleItems" :key="item.id" :class="{selected: actingId === item.id}">
              <td>
                <div>{{ item.account_snapshot || item.accountSnapshot || '—' }}</div>
                <div class="muted small-text">{{ item.auth_label || item.authLabel || '' }}</div>
              </td>
              <td>{{ item.provider || '—' }}</td>
              <td>{{ item.auth_file_name || item.authFileName || '—' }}</td>
              <td><span :class="['status-badge', actionBadgeClass(item.action_type || item.actionType)]">{{ actionLabel(item.action_type || item.actionType) }}</span></td>
              <td>{{ item.error_kind || item.errorKind || item.header_error_kind || item.headerErrorKind || '—' }}</td>
              <td>{{ item.error_code || item.errorCode || item.header_error_code || item.headerErrorCode || '—' }}</td>
              <td><span :class="['status-badge', item.status === 'pending' ? 'bad' : 'good']">{{ statusLabel(item.status) }}</span></td>
              <td class="small-text">{{ formatMs(item.triggered_at_ms || item.triggeredAtMs) }}</td>
              <td>
                <div class="config-actions-bar" style="padding:0;gap:4px">
                  <button v-if="item.status === 'pending'" class="btn primary" size="xs" @click="act(item.id, 'enable')" :disabled="busy">启用</button>
                  <button v-if="item.status === 'pending'" class="btn" @click="act(item.id, 'ignore')" :disabled="busy">忽略</button>
                  <button v-if="item.status === 'pending'" class="btn danger" @click="act(item.id, 'delete')" :disabled="busy">删除</button>
                  <button v-if="item.status === 'pending'" class="btn" @click="act(item.id, 'resolve')" :disabled="busy">解决</button>
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
import DataCard from './DataCard.vue';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const items = ref([]);
const loading = ref(false);
const busy = ref(false);
const error = ref('');
const filter = ref('pending');
const search = ref('');
const actingId = ref(null);

const visibleItems = computed(() => {
  const term = search.value.trim().toLowerCase();
  if(!term) return items.value;
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

onMounted(() => { if(props.ready) loadCandidates(); });

async function loadCandidates(){
  if(!props.ready) return;
  loading.value = true;
  error.value = '';
  try{
    const params = {status: filter.value === 'all' ? '' : filter.value, limit: 200};
    const qs = params.status ? `status=${encodeURIComponent(params.status)}&limit=${params.limit}` : `limit=${params.limit}`;
    const resp = await props.proxyCall({
      method:'GET',
      path:'/v0/management/account-action-candidates',
      query: qs,
    });
    items.value = resp?.items || [];
  }catch(e){
    error.value = e.message || String(e);
  }finally{
    loading.value = false;
  }
}
async function act(id, action){
  busy.value = true;
  actingId.value = id;
  try{
    const paths = {
      enable: `/v0/management/account-action-candidates/${encodeURIComponent(id)}/enable`,
      ignore: `/v0/management/account-action-candidates/${encodeURIComponent(id)}/ignore`,
      resolve: `/v0/management/account-action-candidates/${encodeURIComponent(id)}/resolve`,
      delete: `/v0/management/account-action-candidates/${encodeURIComponent(id)}`,
    };
    await props.proxyCall({method: action === 'delete' ? 'DELETE' : 'POST', path: paths[action]});
    await loadCandidates();
  }catch(e){
    error.value = e.message || String(e);
  }finally{
    busy.value = false;
    actingId.value = null;
  }
}
function actionLabel(a){ return {delete:'删除', reauth:'重新认证', enable:'启用', keep:'保留', review:'审查'}[a] || a || '—'; }
function actionBadgeClass(a){ return a === 'delete' ? 'bad' : a === 'reauth' ? 'warn' : 'good'; }
function statusLabel(s){ return {pending:'待处理', ignored:'已忽略', resolved:'已解决', deleted:'已删除'}[s] || s || '—'; }
function formatMs(ms){ if(!ms) return '—'; return new Date(Number(ms)).toLocaleString('zh-CN', {hour12:false}); }
defineExpose({ refresh: loadCandidates });
</script>