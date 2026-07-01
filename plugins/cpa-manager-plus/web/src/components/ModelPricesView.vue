<template>
  <section class="monitoring-page">
    <div class="card model-prices-header">
      <div class="filterbar-row">
        <button class="btn" @click="refresh(true)" :disabled="loading">{{ loading ? '加载中…' : '刷新' }}</button>
        <button class="btn" @click="addManualPrice">添加手动价格</button>
      </div>
    </div>

    <section v-if="error" class="notice error">{{ error }}</section>

    <DataCard title="已配置单价模型" :subtitle="`${modelCount} 个模型`">
      <SimpleTable :rows="modelRows" :columns="columns" />
    </DataCard>

    <div v-if="editingModel" class="drawer-backdrop" @click.self="editingModel = null">
      <div class="modal-dialog card drawer">
        <div class="drawer-head">
          <div><h2>{{ editingModel.isNew ? '新增单价' : '编辑单价' }}</h2><p class="muted">{{ editingModel.model || '新模型' }}</p></div>
          <button class="btn" @click="editingModel = null">关闭</button>
        </div>
        <div class="config-form-grid">
          <label class="config-field">
            <span class="config-field-label">模型名称</span>
            <input v-model.trim="editingModel.model" class="control" :disabled="!editingModel.isNew" />
          </label>
          <label class="config-field">
            <span class="config-field-label">Prompt 价格 (per 1M tokens)</span>
            <input v-model.number="editingModel.prompt" type="number" step="0.01" min="0" class="control" />
          </label>
          <label class="config-field">
            <span class="config-field-label">Completion 价格 (per 1M tokens)</span>
            <input v-model.number="editingModel.completion" type="number" step="0.01" min="0" class="control" />
          </label>
          <label class="config-field">
            <span class="config-field-label">Cache 价格 (per 1M tokens)</span>
            <input v-model.number="editingModel.cache" type="number" step="0.01" min="0" class="control" />
          </label>
          <label class="config-field">
            <span class="config-field-label">Cache Read 价格</span>
            <input v-model.number="editingModel.cacheRead" type="number" step="0.01" min="0" class="control" />
          </label>
          <label class="config-field">
            <span class="config-field-label">Cache Creation 价格</span>
            <input v-model.number="editingModel.cacheCreation" type="number" step="0.01" min="0" class="control" />
          </label>
        </div>
        <div class="config-actions-bar">
          <button class="btn primary" @click="savePrice" :disabled="saving">{{ saving ? '保存中…' : '保存' }}</button>
          <button class="btn" @click="editingModel = null">取消</button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, defineComponent, h, onMounted, ref } from 'vue';
import DataCard from './DataCard.vue';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const data = ref({});
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const editingModel = ref(null);

const modelRows = computed(() => {
  const prices = data.value?.prices || data.value || {};
  return Object.entries(prices).map(([model, price]) => ({
    model,
    prompt: price.prompt ?? price.Prompt ?? '',
    completion: price.completion ?? price.Completion ?? '',
    cache: price.cache ?? price.Cache ?? '',
    cacheRead: price.cacheRead ?? price.CacheRead ?? '',
    cacheCreation: price.cacheCreation ?? price.CacheCreation ?? '',
    source: price.source ?? price.Source ?? '',
    sourceModelId: price.sourceModelId ?? price.SourceModelId ?? '',
  })).sort((a,b) => a.model.localeCompare(b.model));
});
const modelCount = computed(() => modelRows.value.length);

const columns = [
  ['model','模型'], ['prompt','Prompt/1M','money'], ['completion','Completion/1M','money'], ['cache','Cache/1M','money'],
  ['cacheRead','Cache Read','money'], ['cacheCreation','Cache Creation','money'], ['source','来源'],
];

onMounted(() => { if(props.ready) refresh(true); });

async function refresh(force=false){
  if(!props.ready) return;
  if(loading.value && !force) return;
  loading.value = true;
  error.value = '';
  try{
    data.value = await props.proxyCall({method:'GET', path:'/v0/management/model-prices'});
  }catch(e){
    error.value = e.message || String(e);
  }finally{
    loading.value = false;
  }
}
function addManualPrice(){
  editingModel.value = {isNew: true, model:'', prompt:0, completion:0, cache:0, cacheRead:0, cacheCreation:0};
}
async function savePrice(){
  if(!editingModel.value || !editingModel.value.model) return;
  saving.value = true;
  try{
    const prices = {...(data.value?.prices || data.value || {})};
    prices[editingModel.value.model] = {
      prompt: Number(editingModel.value.prompt) || 0,
      completion: Number(editingModel.value.completion) || 0,
      cache: Number(editingModel.value.cache) || 0,
      cacheRead: Number(editingModel.value.cacheRead) || 0,
      cacheCreation: Number(editingModel.value.cacheCreation) || 0,
    };
    await props.proxyCall({method:'PUT', path:'/v0/management/model-prices', body:{prices}});
    editingModel.value = null;
    await refresh(true);
  }catch(e){
    error.value = e.message || String(e);
  }finally{
    saving.value = false;
  }
}
defineExpose({ refresh });

const SimpleTable = defineComponent({
  props: { rows:{type:Array, default:()=>[]}, columns:{type:Array, default:()=>[]} },
  setup(props){
    return () => {
      if(!props.rows.length) return h('div', {class:'empty'}, '暂无数据');
      return h('div', {class:'table-wrap monitor-table'},
        h('table', [
          h('thead', h('tr', props.columns.map(c => h('th', c[1])))),
          h('tbody', props.rows.slice(0, 200).map((row, idx) =>
            h('tr', {key:idx, class:'clickable', onClick: () => {} },
              props.columns.map(c => h('td', renderCell(row[c[0]], c[2])))
            )
          ))
        ])
      );
    };
  }
});
function renderCell(v, type){
  if(type === 'money') return v == null ? '—' : '$' + Number(v).toFixed(4);
  if(v == null || v === '') return '—';
  return String(v);
}
</script>