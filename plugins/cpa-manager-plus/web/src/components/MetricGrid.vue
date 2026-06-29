<template>
  <div class="metric-grid">
    <div v-for="card in cards" :key="card.label" class="card glass metric-card">
      <div class="metric-label">{{ card.label }}</div>
      <div class="metric-value">{{ format(card.value) }}</div>
      <div v-if="card.sub" class="metric-sub">{{ card.sub }}</div>
    </div>
  </div>
</template>

<script setup>
import { num } from '../utils/data.js';

defineProps({ cards: { type: Array, default: () => [] } });
const format = num;
</script>
