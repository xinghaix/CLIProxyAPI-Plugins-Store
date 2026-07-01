<template>
  <div v-if="open" class="modal-backdrop" role="presentation" @click.self="onCancel">
    <div class="modal-dialog card confirm-modal" role="alertdialog" :aria-labelledby="titleId">
      <div class="modal-head">
        <div>
          <h2 :id="titleId">{{ title }}</h2>
          <p v-if="message" class="muted confirm-message">{{ message }}</p>
        </div>
      </div>
      <div class="confirm-actions">
        <button type="button" class="btn" @click="onCancel">{{ cancelLabel }}</button>
        <button
          type="button"
          :class="['btn', variant === 'danger' ? 'danger' : 'primary']"
          @click="onConfirm"
        >
          {{ confirmLabel }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { useId } from 'vue';

defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, default: '确认' },
  message: { type: String, default: '' },
  confirmLabel: { type: String, default: '确定' },
  cancelLabel: { type: String, default: '取消' },
  variant: { type: String, default: 'primary' },
});

const emit = defineEmits(['confirm', 'cancel']);

const titleId = useId();

function onConfirm() {
  emit('confirm');
}

function onCancel() {
  emit('cancel');
}
</script>