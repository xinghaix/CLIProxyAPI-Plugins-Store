<template>
  <div :class="['metric-grid', $attrs.class]">
    <div
      v-for="card in cards"
      :key="card.key || card.label"
      class="metric metric-card"
      :class="card.accent ? `metric-accent-${card.accent}` : ''"
    >
      <div class="metric-label">
        <span v-if="card.iconClass" :class="['metric-icon', card.iconClass]" aria-hidden="true"></span>
        {{ card.label }}
      </div>
      <div class="metric-value" :title="card.valueTitle || undefined">{{ format(card.value) }}</div>
      <div v-if="card.sub" class="metric-sub">{{ card.sub }}</div>
    </div>
  </div>
</template>

<script setup>
import { num } from '../utils/data.js';

defineOptions({ inheritAttrs: false });

defineProps({ cards: { type: Array, default: () => [] } });
const format = num;
</script>
