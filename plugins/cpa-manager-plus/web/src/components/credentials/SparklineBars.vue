<template>
  <div
    class="cred-spark"
    :class="{ 'cred-spark-status': isStatus }"
    :title="title"
    role="img"
    :aria-label="title || ariaFallback"
  >
    <i
      v-for="(slot, index) in slots"
      :key="index"
      :class="slotClass(slot)"
      :style="isStatus ? undefined : { height: `${Math.max(8, Number(slot) || 0)}%` }"
    ></i>
  </div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  /** Volume mode: numeric heights. Ignored when `statuses` is provided. */
  values: { type: Array, default: () => [] },
  /**
   * Status-track mode (CPA-Manager-Plus AccountLatestRequest):
   * 'ok' | 'fail' | null (empty pad). Rendered oldest→newest.
   */
  statuses: { type: Array, default: null },
  title: { type: String, default: '' },
});

const isStatus = computed(() => Array.isArray(props.statuses));

const slots = computed(() => {
  if (isStatus.value) {
    return props.statuses.length ? props.statuses : Array.from({ length: 8 }, () => null);
  }
  const nums = (props.values || []).map((v) => Number(v) || 0);
  const max = Math.max(0, ...nums);
  if (max <= 0) return nums.map(() => 0);
  return nums.map((n) => Math.round((n / max) * 100));
});

const ariaFallback = computed(() => {
  if (!isStatus.value) return '';
  const ok = slots.value.filter((s) => s === 'ok').length;
  const fail = slots.value.filter((s) => s === 'fail').length;
  return `${ok} ok · ${fail} fail`;
});

function slotClass(slot) {
  if (!isStatus.value) return { idle: Number(slot) <= 0 };
  if (slot === 'ok') return { ok: true };
  if (slot === 'fail') return { fail: true };
  return { idle: true };
}
</script>
