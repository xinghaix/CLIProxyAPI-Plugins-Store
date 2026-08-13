<template>
  <div :class="['card', { glass }]">
    <div v-if="title || subtitle" class="section-title">
      <h2 v-if="title">{{ title }}</h2>
      <span v-if="subtitle">{{ subtitle }}</span>
    </div>
    <slot />
  </div>
</template>

<script setup>
defineProps({
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' },
  glass: { type: Boolean, default: false },
});
</script>
