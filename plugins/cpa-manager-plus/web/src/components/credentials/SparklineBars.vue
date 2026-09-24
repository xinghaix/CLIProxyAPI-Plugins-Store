<template>
  <div class="cred-spark" :title="title" role="img" :aria-label="title">
    <i
      v-for="(value, index) in normalized"
      :key="index"
      :style="{ height: `${Math.max(8, value)}%` }"
      :class="{ idle: value <= 0 }"
    ></i>
  </div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  values: { type: Array, default: () => [] },
  title: { type: String, default: '' },
});

const normalized = computed(() => {
  const nums = (props.values || []).map((v) => Number(v) || 0);
  const max = Math.max(0, ...nums);
  if (max <= 0) return nums.map(() => 0);
  return nums.map((n) => Math.round((n / max) * 100));
});
</script>
