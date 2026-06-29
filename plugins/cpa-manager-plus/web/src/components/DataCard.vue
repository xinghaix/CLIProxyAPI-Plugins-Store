<template>
  <div :class="['card', { glass }]">
    <div class="section-title">
      <h2>{{ title }}</h2>
      <span>{{ subtitle }}</span>
    </div>
    <slot />
  </div>
</template>

<script setup>
defineProps({
  title: { type: String, required: true },
  subtitle: { type: String, default: '' },
  glass: { type: Boolean, default: false },
});
</script>
