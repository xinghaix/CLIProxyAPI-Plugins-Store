<template>
  <div v-if="open" class="modal-backdrop" role="presentation" @click.self="onCancel">
    <div class="modal-dialog card confirm-modal" role="alertdialog" :aria-labelledby="titleId">
      <div class="modal-head">
        <div>
          <h2 :id="titleId">{{ resolvedTitle }}</h2>
          <p v-if="message" class="muted confirm-message">{{ message }}</p>
        </div>
      </div>
      <div class="confirm-actions">
        <button type="button" class="btn" @click="onCancel">{{ resolvedCancelLabel }}</button>
        <button
          type="button"
          :class="['btn', variant === 'danger' ? 'danger' : 'primary']"
          @click="onConfirm"
        >
          {{ resolvedConfirmLabel }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, useId } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, default: '' },
  message: { type: String, default: '' },
  confirmLabel: { type: String, default: '' },
  cancelLabel: { type: String, default: '' },
  variant: { type: String, default: 'primary' },
});

const emit = defineEmits(['confirm', 'cancel']);
const { t } = useI18n();

const titleId = useId();
const resolvedTitle = computed(() => props.title || t('common.confirm'));
const resolvedConfirmLabel = computed(() => props.confirmLabel || t('common.confirm'));
const resolvedCancelLabel = computed(() => props.cancelLabel || t('common.cancel'));

function onConfirm() {
  emit('confirm');
}

function onCancel() {
  emit('cancel');
}
</script>
