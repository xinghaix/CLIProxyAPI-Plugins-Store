<template>
  <div>
    <div v-if="!normalized.length" class="empty">暂无数据</div>
    <div v-else class="table-wrap">
      <table>
        <thead>
          <tr><th v-for="col in columns" :key="col">{{ col }}</th></tr>
        </thead>
        <tbody>
          <tr v-for="(row, idx) in normalized" :key="idx">
            <td v-for="col in columns" :key="col">{{ formatCell(row[col]) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { formatCell } from '../utils/data.js';

const props = defineProps({
  rows: { type: Array, default: () => [] },
  preferredKeys: { type: Array, default: () => [] },
});

const normalized = computed(() => props.rows.slice(0, 30).map(row => (row && typeof row === 'object') ? row : {value: row}));
const columns = computed(() => {
  if(!normalized.value.length) return [];
  const keys = props.preferredKeys.filter(k => normalized.value.some(r => r[k] != null));
  return keys.length ? keys : Object.keys(normalized.value[0] || {}).slice(0, 6);
});
</script>
