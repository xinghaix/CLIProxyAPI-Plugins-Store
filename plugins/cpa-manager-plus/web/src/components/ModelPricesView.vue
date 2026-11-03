<template>
  <section class="monitoring-page model-prices-page">
    <!-- ====== Unified Command Bar ====== -->
    <div class="card dash-command-bar mp-command-bar">
      <div class="dash-command-top">
        <div class="inspection-status-info">
          <span :class="['status-badge', syncing || status.running ? '' : lastStatusTone]">
            <i aria-hidden="true"></i>{{ syncStatusText }}
          </span>
          <span :class="['status-badge', settings.enabled ? 'good' : '']">
            <i aria-hidden="true"></i>{{ settings.enabled ? t('modelPrices.autoSyncOn') : t('modelPrices.autoSyncOff') }}
          </span>
          <span class="muted small-text">
            {{ t('modelPrices.syncModels', { count: syncModelCount }) }}
            <template v-if="status.lastSyncAtMs"> · {{ t('modelPrices.lastSync', { time: formatTimestamp(status.lastSyncAtMs, localeRef) }) }}</template>
            <template v-if="lastResult?.trigger"> · {{ triggerLabel(lastResult.trigger) }}</template>
          </span>
        </div>

        <div class="dash-command-right">
          <button
            type="button"
            :class="['btn', 'dash-filter-toggle', { active: showSyncSettings }]"
            @click="showSyncSettings = !showSyncSettings"
          >
            {{ t('modelPrices.settingsTitle') }}
          </button>
          <button class="btn" type="button" @click="addManualPrice" :disabled="!ready">
            {{ t('modelPrices.addManual') }}
          </button>
          <button class="btn" type="button" @click="refresh(true)" :disabled="loading || !ready">
            {{ loading ? t('common.loading') : t('common.refresh') }}
          </button>
          <button class="btn primary" type="button" @click="runSync" :disabled="!ready || syncing || status.running">
            {{ syncing || status.running ? t('modelPrices.syncing') : t('modelPrices.syncNow') }}
          </button>
        </div>
      </div>

      <!-- Collapsible Auto-Sync Settings & Source Health Drawer -->
      <div v-if="showSyncSettings" class="mp-settings-drawer">
        <div class="mp-settings-row">
          <label class="mp-inline-field">
            <span class="config-field-label">{{ t('modelPrices.enableAutoSync') }}</span>
            <button
              type="button"
              :class="['toggle-switch', { on: settingsDraft.enabled }]"
              :disabled="!ready || settingsSaving || !syncApiAvailable.settings"
              @click="settingsDraft.enabled = !settingsDraft.enabled"
            >
              <span class="toggle-knob"></span>
            </button>
            <small class="muted">{{ settingsDraft.enabled ? t('common.enabled') : t('modelPrices.defaultOff') }}</small>
          </label>

          <label class="mp-inline-field">
            <span class="config-field-label">{{ t('modelPrices.intervalHours') }}</span>
            <input
              v-model.number="settingsDraft.intervalHours"
              type="number"
              :min="MIN_INTERVAL_HOURS"
              :max="MAX_INTERVAL_HOURS"
              step="1"
              class="control compact"
              style="width: 96px"
              :disabled="!ready || settingsSaving || !syncApiAvailable.settings"
            />
            <small class="muted">{{ t('modelPrices.intervalHint', { min: MIN_INTERVAL_HOURS, max: MAX_INTERVAL_HOURS, default: DEFAULT_SYNC_SETTINGS.intervalHours }) }}</small>
          </label>

          <div class="mp-inline-field">
            <span class="dash-kpi-chip">{{ t('modelPrices.protectManual') }}: {{ t('common.enabled') }}</span>
            <small class="muted">{{ t('modelPrices.protectManualHint') }}</small>
          </div>

          <button
            type="button"
            class="btn primary"
            :disabled="!ready || settingsSaving || !settingsDirty"
            @click="saveSettings"
          >
            {{ settingsSaving ? t('modelPrices.savingSettings') : t('modelPrices.saveSettings') }}
          </button>
        </div>

        <div v-if="sourceResults.length" class="model-prices-sources">
          <div v-for="src in sourceResults" :key="src.source" class="source-pill">
            <span :class="['status-badge', src.ok ? 'good' : 'bad']">
              <i aria-hidden="true"></i>{{ formatSourceLabel(src.source) }}
            </span>
            <span v-if="src.ok" class="muted small-text">
              {{ t('modelPrices.sourceOk', { models: src.models || 0, matched: src.matched || 0, applied: src.applied || 0 }) }}
            </span>
            <span v-else class="bad-text small-text">
              {{ src.error ? t('modelPrices.sourceFailedWithError', { error: src.error }) : t('modelPrices.sourceFailed') }}
            </span>
          </div>
        </div>

        <details class="inspection-info-note">
          <summary>{{ t('modelPrices.infoTitle') }}</summary>
          <ul class="inspection-info-list">
            <li><strong>{{ t('modelPrices.info.scope') }}</strong>: {{ t('modelPrices.info.scopeText') }}</li>
            <li><strong>{{ t('modelPrices.info.dualSource') }}</strong>: {{ t('modelPrices.info.dualSourceText') }}</li>
            <li><strong>{{ t('modelPrices.info.manualProtect') }}</strong>: {{ t('modelPrices.info.manualProtectText') }}</li>
            <li><strong>{{ t('modelPrices.info.autoSync') }}</strong>: {{ t('modelPrices.info.autoSyncText') }}</li>
          </ul>
        </details>
      </div>
    </div>

    <div v-if="!ready" class="notice warn">{{ t('modelPrices.missingKey') }}</div>
    <div v-if="error" class="notice error">{{ error }}</div>
    <div v-if="syncNotice" class="notice">{{ syncNotice }}</div>

    <!-- ====== 4-Card High-Density KPI Strip ====== -->
    <section class="dash-kpi-strip">
      <article class="dash-kpi-card clickable" @click="rowFilter = 'all'">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('modelPrices.summary.syncTargets') }}</span>
          <span class="dash-kpi-chip">{{ t('modelPrices.filters.saved') }} {{ savedRowCount }}</span>
        </div>
        <div class="dash-kpi-value">{{ allRows.length }}</div>
        <div class="dash-kpi-foot">
          <span>{{ t('windowKeeper.ext.officialTier') }} <b>{{ officialTieredRowCount }}</b></span>
          <span class="dash-kpi-sep">·</span>
          <span>{{ t('prices.source.manual') }} <b>{{ manualRowCount }}</b></span>
        </div>
      </article>

      <article class="dash-kpi-card clickable" @click="rowFilter = missingRowCount > 0 ? 'missing' : 'all'">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('modelPrices.filters.missing') }}</span>
          <span class="dash-kpi-chip">{{ t('modelPrices.summary.unmatched') }} {{ summary.unmatched }}</span>
        </div>
        <div :class="['dash-kpi-value', missingRowCount > 0 ? 'warn-text' : 'good-text']">
          {{ missingRowCount }}
        </div>
        <div class="dash-kpi-foot">
          <span>{{ t('windowKeeper.ext.unpricedSub') }}</span>
        </div>
      </article>

      <article class="dash-kpi-card clickable" @click="rowFilter = pendingCandidates.length > 0 ? 'candidates' : 'all'">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('modelPrices.summary.pendingCandidates') }}</span>
          <span class="dash-kpi-chip">{{ t('windowKeeper.ext.modelsUnit', { count: groupedCandidates.length }) }}</span>
        </div>
        <div :class="['dash-kpi-value', pendingCandidates.length > 0 ? 'warn-text' : '']">
          {{ pendingCandidates.length }}
        </div>
        <div class="dash-kpi-foot">
          <span>{{ t('modelPrices.summary.pendingCandidatesSub') }}</span>
        </div>
      </article>

      <article class="dash-kpi-card clickable" @click="rowFilter = 'saved'">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('modelPrices.summary.autoUpdated') }}</span>
          <span class="dash-kpi-chip">{{ t('modelPrices.summary.protectedSkipped') }} {{ summary.protectedSkipped }}</span>
        </div>
        <div class="dash-kpi-value">{{ autoSyncedRowCount }}</div>
        <div class="dash-kpi-foot">
          <span>Models.dev / LiteLLM / OpenRouter</span>
        </div>
      </article>
    </section>

    <!-- ====== Grouped Pending Candidates Section ====== -->
    <DataCard
      v-if="groupedCandidates.length"
      :title="t('modelPrices.candidatesTitle')"
      :subtitle="t('modelPrices.candidatesSubtitle', { count: pendingCandidates.length })"
    >
      <div class="mp-candidate-groups">
        <article v-for="group in groupedCandidates" :key="group.localModel" class="mp-candidate-card">
          <div class="mp-candidate-head">
            <div class="mp-candidate-title">
              <strong>{{ group.localModel }}</strong>
              <span v-if="group.usage?.calls" class="dash-kpi-chip">
                {{ fmtCompact(group.usage.calls) }} 请求 · {{ fmtCompact(group.usage.total_tokens) }} tok
              </span>
            </div>
            <button
              class="btn btn-xs"
              type="button"
              :disabled="confirmingCandidate || dismissingCandidates"
              @click="dismissSingleModelCandidate(group.localModel)"
            >
              {{ t('windowKeeper.ext.dismissModel') }}
            </button>
          </div>

          <div class="mp-candidate-options">
            <div v-for="item in group.items" :key="item.key" class="mp-candidate-option">
              <label class="mp-candidate-check">
                <input
                  type="checkbox"
                  :checked="selectedCandidateKeys.has(item.key)"
                  @change="toggleCandidateKey(item.key, $event.target.checked)"
                />
                <span class="source-tag">{{ formatSourceLabel(item.cand.source) }}</span>
                <strong class="mono-cell">{{ item.cand.sourceModelId }}</strong>
              </label>

              <div class="mp-candidate-rates mono-cell">
                <span>入 <b>{{ fmtPrice(item.cand.prompt ?? item.cand.price?.prompt) }}</b></span>
                <span>·</span>
                <span>出 <b>{{ fmtPrice(item.cand.completion ?? item.cand.price?.completion) }}</b></span>
                <template v-if="Number(item.cand.cacheRead ?? item.cand.cache ?? item.cand.price?.cacheRead ?? item.cand.price?.cache ?? 0) > 0">
                  <span>·</span>
                  <span>缓 <b>{{ fmtPrice(item.cand.cacheRead ?? item.cand.cache ?? item.cand.price?.cacheRead ?? item.cand.price?.cache) }}</b></span>
                </template>
              </div>

              <div class="mp-candidate-actions">
                <span class="muted small-text">{{ item.cand.reason || '模糊匹配' }}</span>
                <button
                  class="btn btn-xs primary"
                  type="button"
                  :disabled="confirmingCandidate"
                  @click="confirmSingleCandidate(item.cand)"
                >
                  {{ t('modelPrices.confirm') }}
                </button>
                <button
                  class="btn btn-xs"
                  type="button"
                  @click="editFromCandidate(item.cand)"
                >
                  {{ t('modelPrices.edit') }}
                </button>
              </div>
            </div>
          </div>
        </article>
      </div>

      <div class="config-actions-bar">
        <button
          class="btn primary"
          type="button"
          :disabled="!selectedCandidateList.length || confirmingCandidate"
          @click="confirmSelectedCandidates"
        >
          {{ confirmingCandidate ? t('modelPrices.confirming') : t('modelPrices.confirmSelected', { count: selectedCandidateList.length }) }}
        </button>
        <button
          class="btn"
          type="button"
          :disabled="confirmingCandidate || dismissingCandidates"
          @click="dismissAllCandidates"
        >
          {{ t('modelPrices.dismissCandidates') }}
        </button>
      </div>
    </DataCard>

    <!-- ====== Filter & Sort Toolbar + Responsive Price Card Grid ====== -->
    <div class="card mp-Catalog-shell">
      <div class="mp-toolbar">
        <div class="segmented-control mp-filter-pills" role="tablist">
          <button
            v-for="f in ['all', 'saved', 'official', 'missing', 'candidates']"
            :key="f"
            type="button"
            :class="['segment-btn', { active: rowFilter === f }]"
            @click="rowFilter = f"
          >
            {{ filterLabel(f) }}
            <span class="segment-count">{{ filterCount(f) }}</span>
          </button>
        </div>

        <div class="mp-toolbar-right">
          <input
            v-model.trim="searchQuery"
            class="control compact mp-search-input"
            :placeholder="t('modelPrices.searchPlaceholder')"
          />
          <select v-model="sortMode" class="control compact">
            <option value="usage">{{ t('windowKeeper.ext.sortUsage') }}</option>
            <option value="missingFirst">{{ t('windowKeeper.ext.sortMissingFirst') }}</option>
            <option value="name">{{ t('windowKeeper.ext.sortName') }}</option>
            <option value="updated">{{ t('windowKeeper.ext.sortUpdated') }}</option>
          </select>
        </div>
      </div>

      <div v-if="visibleRows.length" class="table-wrap mp-list-table-wrap">
        <table class="mp-list-table">
          <thead>
            <tr>
              <th>{{ t('modelPrices.columns.model') }}</th>
              <th>{{ t('modelPrices.columns.source') }}</th>
              <th>{{ t('modelPrices.columns.promptPer1M') }}</th>
              <th>{{ t('modelPrices.columns.completionPer1M') }}</th>
              <th>{{ t('modelPrices.columns.cacheRead') }}</th>
              <th>{{ t('modelPrices.columns.cacheCreation') }}</th>
              <th>{{ t('windowKeeper.ext.callsLabel') }} / {{ t('modelPrices.columns.sourceModel') }}</th>
              <th class="right">{{ t('modelPrices.columns.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="row in visibleRows"
              :key="row.model"
              :class="['mp-list-row', 'clickable', { 'is-missing': row.missing, 'is-official': !!row.officialSchedule, selected: editingModel?.model === row.model }]"
              @click="startEdit(row)"
            >
              <td>
                <div class="mp-list-model-cell">
                  <strong class="mp-model-name" :title="row.model">{{ row.model }}</strong>
                  <span v-if="row.usage?.provider" class="dash-kpi-chip">{{ row.usage.provider }}</span>
                </div>
              </td>
              <td>
                <div class="mp-card-badges">
                  <span v-if="row.officialSchedule" class="source-tag source-official">
                    {{ t('windowKeeper.ext.officialTier') }}
                  </span>
                  <span v-if="row.source" :class="['source-tag', row.source === 'manual' ? 'source-manual' : '']">
                    {{ formatSourceLabel(row.source) }}
                  </span>
                  <span v-else-if="!row.officialSchedule && row.hasCandidate" class="warn-text small-text">
                    {{ t('modelPrices.needsConfirm') }}
                  </span>
                  <span v-else-if="row.missing" class="bad-text small-text">
                    {{ t('modelPrices.notSet') }}
                  </span>
                </div>
              </td>
              <td class="mono-cell">
                <strong>{{ displayRowPrompt(row) }}</strong>
              </td>
              <td class="mono-cell">
                <strong>{{ displayRowCompletion(row) }}</strong>
              </td>
              <td class="mono-cell">
                {{ displayRowCacheRead(row) }}
              </td>
              <td class="mono-cell">
                {{ row.hasSavedFlat && Number(row.cacheCreation || 0) > 0 ? fmtPrice(row.cacheCreation) : EMPTY_VALUE }}
              </td>
              <td>
                <div class="mp-card-meta muted small-text">
                  <span v-if="row.usage?.calls">
                    <b>{{ fmtCompact(row.usage.calls) }}</b> · {{ fmtCompact(row.usage.total_tokens) }} tok
                  </span>
                  <span v-else>{{ t('windowKeeper.ext.noLocalCalls') }}</span>
                  <template v-if="row.sourceModelId">
                    <span>·</span>
                    <span class="mono-cell" :title="row.sourceModelId">{{ row.sourceModelId }}</span>
                  </template>
                  <template v-else-if="row.updatedAtMs || row.syncedAtMs">
                    <span>·</span>
                    <span>{{ formatTimestamp(row.updatedAtMs || row.syncedAtMs, localeRef) }}</span>
                  </template>
                </div>
              </td>
              <td class="right" @click.stop>
                <div class="mp-card-actions">
                  <button class="btn btn-xs" type="button" @click="startEdit(row)">
                    {{ row.hasSavedFlat ? t('modelPrices.edit') : t('modelPrices.add') }}
                  </button>
                  <button
                    v-if="row.hasSavedFlat"
                    class="btn btn-xs danger"
                    type="button"
                    :disabled="deleting"
                    @click="confirmDeletePrice(row.model)"
                  >
                    {{ t('common.delete') }}
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="empty">{{ emptyTableText }}</div>
    </div>

    <!-- ====== Edit / Add Price Drawer ====== -->
    <div v-if="editingModel" class="drawer-backdrop" @click.self="closeEditDrawer">
      <aside class="drawer">
        <div class="drawer-header">
          <div>
            <h3>{{ editingModel.isNew ? t('modelPrices.drawerNew') : t('modelPrices.drawerEdit') }}</h3>
            <p class="muted small-text">{{ t('modelPrices.drawerSub', { model: editingModel.model || t('modelPrices.newModel') }) }}</p>
          </div>
          <button class="btn" type="button" @click="closeEditDrawer">{{ t('common.close') }}</button>
        </div>

        <div class="form-grid">
          <label class="field-span-2">
            <span>{{ t('modelPrices.fields.modelName') }}</span>
            <input
              v-model.trim="editingModel.model"
              class="control"
              :disabled="!editingModel.isNew"
              @input="onEditingModelInput"
            />
          </label>

          <section v-if="editingOfficialSchedule" class="field-span-2 mp-drawer-official-box">
            <div class="mp-drawer-official-head">
              <strong>{{ t('windowKeeper.ext.officialTier') }}</strong>
              <span v-if="editingModel.usage?.provider" class="dash-kpi-chip">{{ editingModel.usage.provider }}</span>
            </div>
            <div class="mp-official-bands">
              <div class="mp-band-row">
                <span class="mp-band-tag">Standard ≤{{ fmtThreshold(editingOfficialSchedule) }}</span>
                <span>{{ t('windowKeeper.ext.rateIn') }} <b>{{ fmtRate(editingOfficialSchedule.standard?.Short?.Input) }}</b></span>
                <span>{{ t('windowKeeper.ext.rateCache') }} <b>{{ fmtRate(editingOfficialSchedule.standard?.Short?.CachedInput) }}</b></span>
                <span>{{ t('windowKeeper.ext.rateOut') }} <b>{{ fmtRate(editingOfficialSchedule.standard?.Short?.Output) }}</b></span>
              </div>
              <div v-if="editingOfficialSchedule.standard?.Long" class="mp-band-row muted">
                <span class="mp-band-tag">Standard &gt;{{ fmtThreshold(editingOfficialSchedule) }}</span>
                <span>{{ t('windowKeeper.ext.rateIn') }} <b>{{ fmtRate(editingOfficialSchedule.standard.Long.Input) }}</b></span>
                <span>{{ t('windowKeeper.ext.rateCache') }} <b>{{ fmtRate(editingOfficialSchedule.standard.Long.CachedInput) }}</b></span>
                <span>{{ t('windowKeeper.ext.rateOut') }} <b>{{ fmtRate(editingOfficialSchedule.standard.Long.Output) }}</b></span>
              </div>
              <div v-if="editingOfficialSchedule.fast?.Short" class="mp-band-row mp-band-fast">
                <span class="mp-band-tag">Fast (Priority)</span>
                <span>{{ t('windowKeeper.ext.rateIn') }} <b>{{ fmtRate(editingOfficialSchedule.fast.Short.Input) }}</b></span>
                <span>{{ t('windowKeeper.ext.rateCache') }} <b>{{ fmtRate(editingOfficialSchedule.fast.Short.CachedInput) }}</b></span>
                <span>{{ t('windowKeeper.ext.rateOut') }} <b>{{ fmtRate(editingOfficialSchedule.fast.Short.Output) }}</b></span>
              </div>
            </div>
          </section>

          <section class="field-span-2 price-source-picker">
            <div class="price-source-picker-head">
              <div>
                <strong>{{ t('modelPrices.sourcePicker.title') }}</strong>
                <p class="muted small-text">{{ t('modelPrices.sourcePicker.subtitle') }}</p>
              </div>
              <div class="price-source-picker-actions">
                <button
                  class="btn"
                  type="button"
                  :disabled="!ready || sourceLookupLoading || !String(editingModel.model || '').trim()"
                  @click="lookupModelPriceSources(editingModel.model)"
                >
                  {{ sourceLookupLoading ? t('modelPrices.sourcePicker.loading') : t('modelPrices.sourcePicker.fetch') }}
                </button>
                <button
                  v-if="sourceLookupOptions.length"
                  class="btn primary"
                  type="button"
                  :disabled="!selectedSourceOption"
                  @click="applySelectedSourceToDraft"
                >
                  {{ t('modelPrices.sourcePicker.apply') }}
                </button>
              </div>
            </div>

            <div v-if="sourceLookupError" class="small-text bad-text">{{ sourceLookupError }}</div>
            <div v-else-if="sourceLookupLoaded && !sourceLookupOptions.length" class="small-text muted">
              {{ t('modelPrices.sourcePicker.empty') }}
            </div>

            <div v-if="sourceLookupOptions.length" class="price-source-option-list">
              <label
                v-for="opt in sourceLookupOptions"
                :key="opt.key"
                :class="['price-source-option', { selected: selectedSourceKey === opt.key }]"
              >
                <input
                  v-model="selectedSourceKey"
                  type="radio"
                  name="model-price-source"
                  :value="opt.key"
                />
                <span class="source-tag">{{ formatSourceLabel(opt.source) }}</span>
                <span class="mono-cell">{{ opt.sourceModelId }}</span>
                <span class="muted small-text">{{ formatSourcePriceSummary(opt) }}</span>
              </label>
            </div>
          </section>

          <label>
            <span>{{ t('modelPrices.fields.prompt') }}</span>
            <input v-model.number="editingModel.prompt" type="number" step="0.0001" min="0" class="control" />
          </label>
          <label>
            <span>{{ t('modelPrices.fields.completion') }}</span>
            <input v-model.number="editingModel.completion" type="number" step="0.0001" min="0" class="control" />
          </label>
          <label>
            <span>{{ t('modelPrices.fields.cache') }}</span>
            <input v-model.number="editingModel.cache" type="number" step="0.0001" min="0" class="control" />
          </label>
          <label>
            <span>{{ t('modelPrices.fields.cacheRead') }}</span>
            <input v-model.number="editingModel.cacheRead" type="number" step="0.0001" min="0" class="control" />
          </label>
          <label>
            <span>{{ t('modelPrices.fields.cacheCreation') }}</span>
            <input v-model.number="editingModel.cacheCreation" type="number" step="0.0001" min="0" class="control" />
          </label>
        </div>

        <p class="muted small-text">{{ t('modelPrices.manualNote') }}</p>

        <div class="config-actions-bar">
          <button
            v-if="!editingModel.isNew"
            class="btn danger"
            type="button"
            :disabled="saving || deleting"
            @click="confirmDeletePrice(editingModel.model)"
          >
            {{ deleting ? t('modelPrices.deleting') : t('modelPrices.deletePrice') }}
          </button>
          <button class="btn primary" type="button" @click="savePrice" :disabled="saving || deleting">
            {{ saving ? t('modelPrices.saving') : t('common.save') }}
          </button>
          <button class="btn" type="button" @click="closeEditDrawer">{{ t('common.cancel') }}</button>
        </div>
      </aside>
    </div>

    <ConfirmModal
      :open="confirmOpen"
      :title="confirmTitle"
      :message="confirmMessage"
      :confirmLabel="confirmOkLabel"
      :cancelLabel="confirmCancelLabel"
      :variant="confirmVariant"
      @confirm="finishConfirm(true)"
      @cancel="finishConfirm(false)"
    />
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import ConfirmModal from './ConfirmModal.vue';
import DataCard from './DataCard.vue';
import { localeRef } from '../localeBridge.js';
import { EMPTY_VALUE } from '../utils/localeFormat.js';
import {
  DEFAULT_SYNC_SETTINGS,
  MAX_INTERVAL_HOURS,
  MIN_INTERVAL_HOURS,
  buildConfirmBody,
  buildDeletePriceRequest,
  buildManualPriceEntry,
  buildManualPutBody,
  buildSyncModels,
  extractPricesMap,
  extractUsageModels,
  filterCandidatesWithExistingPrices,
  formatMoneyPer1M,
  formatSourceLabel,
  formatTimestamp,
  normalizeSyncSettings,
  normalizeSyncStatus,
  softProxyCall,
  summarizeLastResult,
  validateIntervalHours,
} from '../utils/priceSync.js';
import {
  buildPriceSourceLookupRequest,
  buildSourcePriceEntry,
  normalizePriceSourceLookup,
} from '../utils/priceSourceLookup.js';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const { t } = useI18n();

const loading = ref(false);
const saving = ref(false);
const deleting = ref(false);
const syncing = ref(false);
const settingsSaving = ref(false);
const confirmingCandidate = ref(false);
const dismissingCandidates = ref(false);
const showSyncSettings = ref(false);
const error = ref('');
const syncNotice = ref('');
const prices = ref({});
const usageModels = ref([]);
const modelUsageMap = ref({});
const officialSchedulesMap = ref({});
const status = ref(normalizeSyncStatus({}));
const settings = ref(normalizeSyncSettings(DEFAULT_SYNC_SETTINGS));
const settingsDraft = reactive({ ...DEFAULT_SYNC_SETTINGS });
const pendingCandidates = ref([]);
const selectedCandidateKeys = ref(new Set());
const editingModel = ref(null);
const sourceLookupLoading = ref(false);
const sourceLookupLoaded = ref(false);
const sourceLookupError = ref('');
const sourceLookupOptions = ref([]);
const selectedSourceKey = ref('');
const rowFilter = ref('all');
const searchQuery = ref('');
const sortMode = ref('usage');

const confirmOpen = ref(false);
const confirmTitle = ref('');
const confirmMessage = ref('');
const confirmOkLabel = ref('');
const confirmCancelLabel = ref('');
const confirmVariant = ref('primary');
let confirmResolve = null;

const syncApiAvailable = reactive({
  status: true,
  settings: true,
  sync: true,
});

let pollTimer = null;

const lastResult = computed(() => status.value?.lastResult || null);

const catalogModels = computed(() =>
  buildSyncModels(prices.value, usageModels.value),
);

const syncModelCount = computed(() => {
  const fromStatus = Number(status.value?.targetModelCount || 0);
  return Math.max(fromStatus, catalogModels.value.length);
});

const candidateModelSet = computed(() => {
  const set = new Set();
  for (const c of pendingCandidates.value) {
    const m = String(c.localModel || c.model || '').trim();
    if (m) set.add(m);
  }
  return set;
});

function resolveOfficialSchedule(model) {
  const raw = String(model || '').trim().toLowerCase();
  if (!raw) return null;
  const map = officialSchedulesMap.value || {};
  if (map[raw]) return map[raw];
  // Match date-pinned suffix like gpt-5.4-2026-03-01
  const base = raw.replace(/-\d{4}-\d{2}-\d{2}$/, '');
  if (base !== raw && map[base]) return map[base];
  return null;
}

const allRows = computed(() => {
  const map = prices.value || {};
  const names = new Set([
    ...Object.keys(map),
    ...catalogModels.value,
    ...Object.keys(modelUsageMap.value || {}),
  ]);
  return Array.from(names)
    .map((m) => String(m || '').trim())
    .filter(Boolean)
    .map((model) => {
      const p = map[model];
      const hasSavedFlat = Boolean(p);
      const officialSchedule = resolveOfficialSchedule(model);
      const missing = !hasSavedFlat && !officialSchedule;
      const usage = modelUsageMap.value[model] || null;
      return {
        model,
        prompt: p?.prompt ?? 0,
        completion: p?.completion ?? 0,
        cache: p?.cache ?? 0,
        cacheRead: p?.cacheRead ?? p?.cache_read ?? 0,
        cacheCreation: p?.cacheCreation ?? p?.cache_creation ?? 0,
        source: p?.source || '',
        sourceModelId: p?.sourceModelId || p?.source_model_id || '',
        syncedAtMs: Number(p?.syncedAtMs || p?.synced_at_ms || 0),
        updatedAtMs: Number(p?.updatedAtMs || p?.updated_at_ms || 0),
        hasSavedFlat,
        officialSchedule,
        missing,
        hasCandidate: candidateModelSet.value.has(model),
        usage,
      };
    });
});

const missingRowCount = computed(() => allRows.value.filter((r) => r.missing).length);
const savedRowCount = computed(() => allRows.value.filter((r) => r.hasSavedFlat).length);
const officialTieredRowCount = computed(() => allRows.value.filter((r) => Boolean(r.officialSchedule)).length);
const manualRowCount = computed(() => allRows.value.filter((r) => r.source === 'manual').length);
const autoSyncedRowCount = computed(() => allRows.value.filter((r) => r.hasSavedFlat && r.source && r.source !== 'manual').length);

const groupedCandidates = computed(() => {
  const groups = new Map();
  pendingCandidates.value.forEach((cand, idx) => {
    const localModel = String(cand.localModel || cand.model || '').trim() || 'unknown';
    if (!groups.has(localModel)) {
      groups.set(localModel, {
        localModel,
        usage: modelUsageMap.value[localModel] || null,
        items: [],
      });
    }
    groups.get(localModel).items.push({
      key: candidateKey(cand, idx),
      cand,
      idx,
    });
  });
  return Array.from(groups.values()).sort((a, b) => Number(b.usage?.calls || 0) - Number(a.usage?.calls || 0));
});

const visibleRows = computed(() => {
  const q = searchQuery.value.trim().toLowerCase();
  const filtered = allRows.value.filter((row) => {
    if (rowFilter.value === 'missing' && !row.missing) return false;
    if (rowFilter.value === 'saved' && !row.hasSavedFlat) return false;
    if (rowFilter.value === 'official' && !row.officialSchedule) return false;
    if (rowFilter.value === 'candidates' && !row.hasCandidate) return false;
    if (!q) return true;
    return (
      row.model.toLowerCase().includes(q)
      || String(row.source || '').toLowerCase().includes(q)
      || String(row.sourceModelId || '').toLowerCase().includes(q)
      || String(row.usage?.provider || '').toLowerCase().includes(q)
    );
  });

  return filtered.sort((a, b) => {
    if (sortMode.value === 'usage') {
      const ca = Number(a.usage?.calls || 0);
      const cb = Number(b.usage?.calls || 0);
      if (cb !== ca) return cb - ca;
      return a.model.localeCompare(b.model);
    }
    if (sortMode.value === 'missingFirst') {
      if (a.missing !== b.missing) return a.missing ? -1 : 1;
      const ca = Number(a.usage?.calls || 0);
      const cb = Number(b.usage?.calls || 0);
      if (cb !== ca) return cb - ca;
      return a.model.localeCompare(b.model);
    }
    if (sortMode.value === 'updated') {
      const ta = Math.max(a.updatedAtMs || 0, a.syncedAtMs || 0);
      const tb = Math.max(b.updatedAtMs || 0, b.syncedAtMs || 0);
      if (tb !== ta) return tb - ta;
      return a.model.localeCompare(b.model);
    }
    return a.model.localeCompare(b.model);
  });
});

const emptyTableText = computed(() => {
  if (!allRows.value.length) return t('modelPrices.emptyNone');
  return t('modelPrices.emptyFilter');
});

const summary = computed(() => summarizeLastResult(lastResult.value, syncModelCount.value));
const sourceResults = computed(() => lastResult.value?.sources || []);

const syncStatusText = computed(() => {
  if (syncing.value || status.value.running) return t('modelPrices.status.running');
  if (status.value.lastError || lastResult.value?.error) return t('modelPrices.status.error');
  if (status.value.lastSuccessAtMs || lastResult.value?.finishedAtMs) return t('modelPrices.status.success');
  return t('modelPrices.status.never');
});

const lastStatusTone = computed(() => {
  if (status.value.lastError || lastResult.value?.error) return 'bad';
  if (status.value.lastSuccessAtMs || status.value.lastSyncAtMs) return 'good';
  return '';
});

const settingsDirty = computed(() => {
  return (
    settingsDraft.enabled !== settings.value.enabled
    || Number(settingsDraft.intervalHours) !== Number(settings.value.intervalHours)
  );
});

const selectedCandidateList = computed(() => {
  return pendingCandidates.value.filter((c, idx) =>
    selectedCandidateKeys.value.has(candidateKey(c, idx)),
  );
});

watch(
  () => props.ready,
  (ready) => {
    if (ready) refresh(true);
  },
);

onMounted(() => {
  if (props.ready) refresh(true);
});

onBeforeUnmount(() => {
  stopPolling();
});

function showConfirm({
  title,
  message = '',
  confirmLabel,
  cancelLabel,
  variant = 'primary',
} = {}) {
  confirmTitle.value = title || t('common.confirm');
  confirmMessage.value = message;
  confirmOkLabel.value = confirmLabel || t('common.confirm');
  confirmCancelLabel.value = cancelLabel || t('common.cancel');
  confirmVariant.value = variant;
  confirmOpen.value = true;
  return new Promise((resolve) => {
    confirmResolve = resolve;
  });
}

function finishConfirm(ok) {
  confirmOpen.value = false;
  const resolve = confirmResolve;
  confirmResolve = null;
  resolve?.(ok);
}

function candidateKey(c, idx) {
  return `${c.localModel}::${c.source}::${c.sourceModelId}::${idx}`;
}

function filterLabel(f) {
  switch (f) {
    case 'missing': return t('modelPrices.filters.missing');
    case 'saved': return t('modelPrices.filters.saved');
    case 'official': return t('windowKeeper.ext.officialTier');
    case 'candidates': return t('modelPrices.filters.candidates');
    default: return t('modelPrices.filters.all');
  }
}

function filterCount(f) {
  if (f === 'missing') return missingRowCount.value;
  if (f === 'saved') return savedRowCount.value;
  if (f === 'official') return officialTieredRowCount.value;
  if (f === 'candidates') return allRows.value.filter((r) => r.hasCandidate).length;
  return allRows.value.length;
}

function triggerLabel(trigger) {
  if (trigger === 'auto' || trigger === 'scheduled') return t('modelPrices.trigger.auto');
  if (trigger === 'manual') return t('modelPrices.trigger.manual');
  return trigger;
}

function fmtPrice(v) {
  return formatMoneyPer1M(v);
}

function fmtRate(v) {
  const n = Number(v);
  if (v == null || !Number.isFinite(n)) return EMPTY_VALUE;
  const trimmed = parseFloat(n.toFixed(4));
  return `$${trimmed}`;
}

function fmtThreshold(schedule) {
  const n = Number(schedule?.context_threshold_tokens || 272000);
  return Math.round(n / 1000) + 'K';
}

function fmtCompact(v) {
  const n = Number(v || 0);
  if (!Number.isFinite(n)) return '0';
  if (Math.abs(n) >= 1e9) return (n / 1e9).toFixed(2) + 'B';
  if (Math.abs(n) >= 1e6) return (n / 1e6).toFixed(1) + 'M';
  if (Math.abs(n) >= 1e3) return (n / 1e3).toFixed(1) + 'K';
  return String(Math.round(n));
}

function effectiveCacheRead(row) {
  const cr = Number(row?.cacheRead || 0);
  if (cr > 0) return cr;
  return Number(row?.cache || 0);
}

function displayRowPrompt(row) {
  if (row?.hasSavedFlat) return fmtPrice(row.prompt);
  if (row?.officialSchedule?.standard?.Short?.Input != null) {
    return fmtRate(row.officialSchedule.standard.Short.Input);
  }
  return EMPTY_VALUE;
}

function displayRowCompletion(row) {
  if (row?.hasSavedFlat) return fmtPrice(row.completion);
  if (row?.officialSchedule?.standard?.Short?.Output != null) {
    return fmtRate(row.officialSchedule.standard.Short.Output);
  }
  return EMPTY_VALUE;
}

function displayRowCacheRead(row) {
  if (row?.hasSavedFlat) return fmtPrice(effectiveCacheRead(row));
  if (row?.officialSchedule?.standard?.Short?.CachedInput != null) {
    return fmtRate(row.officialSchedule.standard.Short.CachedInput);
  }
  return EMPTY_VALUE;
}

const editingOfficialSchedule = computed(() => {
  if (!editingModel.value?.model) return null;
  return editingModel.value.officialSchedule || resolveOfficialSchedule(editingModel.value.model);
});

async function refresh(force = false) {
  if (!props.ready) return;
  if (loading.value && !force) return;
  loading.value = true;
  error.value = '';
  try {
    const [pricesRes, usageRes, statusRes, settingsRes] = await Promise.all([
      softProxyCall(props.proxyCall, { method: 'GET', path: '/v0/management/model-prices' }),
      softProxyCall(props.proxyCall, { method: 'GET', path: '/v0/management/model-prices/usage-summary' }),
      softProxyCall(props.proxyCall, { method: 'GET', path: '/v0/management/model-prices/sync-status' }),
      softProxyCall(props.proxyCall, { method: 'GET', path: '/v0/management/model-prices/sync-settings' }),
    ]);

    if (pricesRes.ok) {
      prices.value = extractPricesMap(pricesRes.data);
    } else {
      error.value = pricesRes.error || t('modelPrices.loadFailed');
    }

    if (usageRes.ok) {
      usageModels.value = extractUsageModels(usageRes.data);
      const uMap = {};
      if (Array.isArray(usageRes.data?.model_usage)) {
        for (const item of usageRes.data.model_usage) {
          if (item?.model) uMap[item.model] = item;
        }
      }
      modelUsageMap.value = uMap;
      if (usageRes.data?.official_schedules && typeof usageRes.data.official_schedules === 'object') {
        officialSchedulesMap.value = usageRes.data.official_schedules;
      }
    }

    if (statusRes.ok) {
      syncApiAvailable.status = true;
      applyStatus(statusRes.data);
    } else if (statusRes.missing) {
      syncApiAvailable.status = false;
    }

    if (settingsRes.ok) {
      syncApiAvailable.settings = true;
      applySettings(settingsRes.data?.settings || settingsRes.data);
    } else if (settingsRes.missing) {
      syncApiAvailable.settings = false;
    }
  } finally {
    loading.value = false;
  }
}

defineExpose({ refresh });

function applyStatus(raw, pricesOverride = prices.value) {
  const normalized = normalizeSyncStatus(raw);
  const filteredCandidates = filterCandidatesWithExistingPrices(
    normalized.lastResult?.candidates || [],
    pricesOverride,
  );
  if (normalized.lastResult) {
    normalized.lastResult.candidates = filteredCandidates;
  }
  status.value = normalized;
  pendingCandidates.value = filteredCandidates;
  selectedCandidateKeys.value = new Set();
  if (normalized.settings) {
    applySettings(normalized.settings);
  }
  if (normalized.running) {
    startPolling();
  } else {
    stopPolling();
  }
}

function applySettings(raw) {
  const normalized = normalizeSyncSettings(raw);
  settings.value = normalized;
  settingsDraft.enabled = normalized.enabled;
  settingsDraft.intervalHours = normalized.intervalHours;
  settingsDraft.protectManual = true;
}

function startPolling() {
  if (pollTimer) return;
  pollTimer = setInterval(async () => {
    const res = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices/sync-status',
    });
    if (!res.ok) {
      stopPolling();
      return;
    }
    applyStatus(res.data);
    if (!status.value.running) {
      stopPolling();
      const pricesRes = await softProxyCall(props.proxyCall, {
        method: 'GET',
        path: '/v0/management/model-prices',
      });
      if (pricesRes.ok) {
        prices.value = extractPricesMap(pricesRes.data);
        applyStatus(res.data, prices.value);
      }
    }
  }, 2000);
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

async function runSync() {
  if (!props.ready || syncing.value || status.value.running) return;
  syncing.value = true;
  error.value = '';
  syncNotice.value = '';
  try {
    const models = catalogModels.value;
    if (!models.length) {
      syncNotice.value = t('modelPrices.noModelsToSync');
      return;
    }

    const res = await softProxyCall(props.proxyCall, {
      method: 'POST',
      path: '/v0/management/model-prices/sync',
      body: { trigger: 'manual', models },
    });

    if (!res.ok) {
      if (res.missing) {
        syncApiAvailable.sync = false;
        error.value = t('modelPrices.syncApiMissing');
      } else {
        error.value = res.error || t('modelPrices.syncFailed');
      }
      return;
    }

    const data = res.data || {};
    if (data.prices) {
      prices.value = extractPricesMap(data);
    } else {
      const pricesRes = await softProxyCall(props.proxyCall, {
        method: 'GET',
        path: '/v0/management/model-prices',
      });
      if (pricesRes.ok) prices.value = extractPricesMap(pricesRes.data);
    }

    if (data.status || data.lastResult || data.result || data.sources || data.candidates) {
      applyStatus(data.status || { lastResult: data.lastResult || data.result || data, running: Boolean(data.running) });
    } else {
      const statusRes = await softProxyCall(props.proxyCall, {
        method: 'GET',
        path: '/v0/management/model-prices/sync-status',
      });
      if (statusRes.ok) applyStatus(statusRes.data);
    }

    const s = summary.value;
    syncNotice.value = t('modelPrices.syncDone', { applied: s.applied, candidates: pendingCandidates.value.length, unmatched: s.unmatched });
  } finally {
    syncing.value = false;
  }
}

async function saveSettings() {
  error.value = '';
  syncNotice.value = '';
  const check = validateIntervalHours(settingsDraft.intervalHours);
  if (!check.valid) {
    error.value = t('modelPrices.intervalInvalid', { min: MIN_INTERVAL_HOURS, max: MAX_INTERVAL_HOURS });
    return;
  }
  settingsSaving.value = true;
  try {
    const res = await softProxyCall(props.proxyCall, {
      method: 'PUT',
      path: '/v0/management/model-prices/sync-settings',
      body: {
        enabled: Boolean(settingsDraft.enabled),
        intervalHours: check.hours,
        interval_hours: check.hours,
        protectManual: true,
        protect_manual: true,
      },
    });
    if (!res.ok) {
      if (res.missing) {
        syncApiAvailable.settings = false;
        error.value = t('modelPrices.settingsApiMissing');
      } else {
        error.value = res.error || t('modelPrices.settingsSaveFailed');
      }
      return;
    }
    applySettings(res.data?.settings || res.data || { enabled: Boolean(settingsDraft.enabled), intervalHours: check.hours });
    syncNotice.value = t('modelPrices.settingsSaved');
  } finally {
    settingsSaving.value = false;
  }
}

function addManualPrice() {
  resetSourceLookup();
  editingModel.value = {
    isNew: true,
    model: '',
    prompt: 0,
    completion: 0,
    cache: 0,
    cacheRead: 0,
    cacheCreation: 0,
  };
}

function cleanRateValue(v) {
  const n = Number(v ?? 0);
  if (!Number.isFinite(n) || n < 0) return 0;
  return parseFloat(n.toFixed(6));
}

function startEdit(row) {
  resetSourceLookup();
  editingModel.value = {
    isNew: false,
    model: row.model,
    prompt: cleanRateValue(row.prompt),
    completion: cleanRateValue(row.completion),
    cache: cleanRateValue(row.cache),
    cacheRead: cleanRateValue(row.cacheRead),
    cacheCreation: cleanRateValue(row.cacheCreation),
    officialSchedule: row.officialSchedule || null,
    usage: row.usage || null,
  };
  lookupModelPriceSources(row.model);
}

function editFromCandidate(c) {
  resetSourceLookup();
  editingModel.value = {
    isNew: false,
    model: c.localModel,
    prompt: cleanRateValue(c.prompt ?? c.price?.prompt),
    completion: cleanRateValue(c.completion ?? c.price?.completion),
    cache: cleanRateValue(c.cache ?? c.price?.cache),
    cacheRead: cleanRateValue(c.cacheRead ?? c.price?.cacheRead),
    cacheCreation: cleanRateValue(c.cacheCreation ?? c.price?.cacheCreation),
  };
  lookupModelPriceSources(c.localModel);
}

function closeEditDrawer() {
  resetSourceLookup();
  editingModel.value = null;
}

function resetSourceLookup() {
  sourceLookupLoading.value = false;
  sourceLookupLoaded.value = false;
  sourceLookupError.value = '';
  sourceLookupOptions.value = [];
  selectedSourceKey.value = '';
}

function onEditingModelInput() {
  sourceLookupLoaded.value = false;
  sourceLookupError.value = '';
  sourceLookupOptions.value = [];
  selectedSourceKey.value = '';
}

const selectedSourceOption = computed(() =>
  sourceLookupOptions.value.find((opt) => opt.key === selectedSourceKey.value) || null,
);

function formatSourcePriceSummary(opt) {
  if (!opt) return '';
  return `in ${fmtPrice(opt.prompt)} · out ${fmtPrice(opt.completion)}`;
}

async function lookupModelPriceSources(model) {
  const req = buildPriceSourceLookupRequest(model);
  if (!props.ready || !req) return;
  sourceLookupLoading.value = true;
  sourceLookupLoaded.value = false;
  sourceLookupError.value = '';
  sourceLookupOptions.value = [];
  selectedSourceKey.value = '';
  try {
    const res = await softProxyCall(props.proxyCall, req);
    if (!res.ok) {
      sourceLookupError.value = res.missing
        ? t('modelPrices.sourcePicker.apiMissing')
        : res.error || t('modelPrices.sourcePicker.failed');
      return;
    }
    const parsed = normalizePriceSourceLookup(res.data);
    const options = parsed.sources.map((s, idx) => ({
      ...s,
      key: `${s.source}::${s.sourceModelId}::${idx}`,
    }));
    sourceLookupOptions.value = options;
    selectedSourceKey.value = options[0]?.key || '';
    sourceLookupLoaded.value = true;
  } finally {
    sourceLookupLoading.value = false;
  }
}

function applySelectedSourceToDraft() {
  if (!editingModel.value || !selectedSourceOption.value) return;
  const entry = buildSourcePriceEntry(editingModel.value.model, selectedSourceOption.value);
  if (!entry) return;
  editingModel.value.prompt = cleanRateValue(entry.price.prompt);
  editingModel.value.completion = cleanRateValue(entry.price.completion);
  editingModel.value.cache = cleanRateValue(entry.price.cache);
  editingModel.value.cacheRead = cleanRateValue(entry.price.cacheRead);
  editingModel.value.cacheCreation = cleanRateValue(entry.price.cacheCreation);
  syncNotice.value = t('modelPrices.sourcePicker.applied', {
    source: formatSourceLabel(entry.selectedSource.source),
  });
}

async function savePrice() {
  const putBody = buildManualPutBody(editingModel.value);
  const entry = buildManualPriceEntry(editingModel.value);
  if (!putBody || !entry) {
    error.value = t('modelPrices.modelNameRequired');
    return;
  }
  saving.value = true;
  error.value = '';
  try {
    await props.proxyCall({
      method: 'PUT',
      path: '/v0/management/model-prices',
      body: putBody,
    });
    resetSourceLookup();
    editingModel.value = null;
    const pricesRes = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices',
    });
    if (pricesRes.ok) {
      prices.value = extractPricesMap(pricesRes.data);
      if (prices.value[entry.model] && !prices.value[entry.model].source) {
        prices.value[entry.model] = {
          ...prices.value[entry.model],
          source: 'manual',
          sourceModelId: '',
          syncedAtMs: 0,
        };
      }
    } else {
      prices.value = { ...prices.value, [entry.model]: entry.price };
    }
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    saving.value = false;
  }
}

async function confirmDeletePrice(model) {
  const value = String(model || '').trim();
  if (!value || deleting.value) return;
  const ok = await showConfirm({
    title: t('modelPrices.deleteTitle'),
    message: t('modelPrices.deleteMessage', { model: value }),
    confirmLabel: t('common.delete'),
    variant: 'danger',
  });
  if (!ok) return;
  await deletePrice(value);
}

async function deletePrice(model) {
  const request = buildDeletePriceRequest(model);
  if (!request || deleting.value) return;
  deleting.value = true;
  error.value = '';
  syncNotice.value = '';
  try {
    const result = await softProxyCall(props.proxyCall, request);
    if (!result.ok) {
      error.value = result.missing
        ? t('modelPrices.deleteApiMissing')
        : result.error || t('modelPrices.deleteFailed');
      return;
    }
    if (editingModel.value?.model === model) {
      resetSourceLookup();
      editingModel.value = null;
    }

    const pricesRes = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices',
    });
    let pricesMap = prices.value;
    if (pricesRes.ok) {
      pricesMap = extractPricesMap(pricesRes.data);
      prices.value = pricesMap;
    } else {
      pricesMap = { ...prices.value };
      delete pricesMap[model];
      prices.value = pricesMap;
    }

    const statusRes = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices/sync-status',
    });
    if (statusRes.ok) {
      applyStatus(statusRes.data, pricesMap);
    } else {
      pendingCandidates.value = filterCandidatesWithExistingPrices(
        pendingCandidates.value,
        pricesMap,
      );
      selectedCandidateKeys.value = new Set();
    }

    syncNotice.value = result.data?.deleted === false
      ? t('modelPrices.deleteAlreadyGone', { model })
      : t('modelPrices.deleteSuccess', { model });
  } finally {
    deleting.value = false;
  }
}

function toggleCandidateKey(key, checked) {
  const next = new Set(selectedCandidateKeys.value);
  if (checked) next.add(key);
  else next.delete(key);
  selectedCandidateKeys.value = next;
}

async function confirmSingleCandidate(candidate) {
  await applyCandidates([candidate]);
}

async function confirmSelectedCandidates() {
  if (!selectedCandidateList.value.length) return;
  await applyCandidates(selectedCandidateList.value);
}

async function applyCandidates(candidates) {
  confirmingCandidate.value = true;
  error.value = '';
  try {
    let appliedCount = 0;
    const confirmedModels = new Set();

    for (const c of candidates) {
      const confirmBody = buildConfirmBody(c);
      if (!confirmBody) {
        error.value = t('modelPrices.candidateMissing');
        return;
      }
      const confirmRes = await softProxyCall(props.proxyCall, {
        method: 'POST',
        path: '/v0/management/model-prices/sync-confirm',
        body: confirmBody,
      });

      if (!confirmRes.ok) {
        error.value = confirmRes.error || t('modelPrices.confirmFailed');
        return;
      }

      appliedCount += 1;
      confirmedModels.add(confirmBody.model);
      if (confirmRes.data?.status) {
        applyStatus(confirmRes.data.status);
      }
    }

    const pricesRes = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices',
    });
    if (pricesRes.ok) prices.value = extractPricesMap(pricesRes.data);

    pendingCandidates.value = pendingCandidates.value.filter(
      (c) => !confirmedModels.has(c.localModel),
    );
    selectedCandidateKeys.value = new Set();
    syncNotice.value = t('modelPrices.confirmed', { count: appliedCount });
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    confirmingCandidate.value = false;
  }
}

async function dismissSingleModelCandidate(localModel) {
  const target = String(localModel || '').trim();
  if (!target) return;
  dismissingCandidates.value = true;
  error.value = '';
  try {
    const res = await softProxyCall(props.proxyCall, {
      method: 'POST',
      path: '/v0/management/model-prices/sync-dismiss',
      body: { models: [target] },
    });
    if (res.ok && res.data?.status) {
      applyStatus(res.data.status);
    } else {
      pendingCandidates.value = pendingCandidates.value.filter(c => c.localModel !== target);
    }
  } finally {
    dismissingCandidates.value = false;
  }
}

async function dismissAllCandidates() {
  const models = Array.from(
    new Set(
      pendingCandidates.value
        .map((c) => String(c.localModel || '').trim())
        .filter(Boolean),
    ),
  );
  dismissingCandidates.value = true;
  error.value = '';
  try {
    if (models.length) {
      const res = await softProxyCall(props.proxyCall, {
        method: 'POST',
        path: '/v0/management/model-prices/sync-dismiss',
        body: { models },
      });
      if (res.ok && res.data?.status) {
        applyStatus(res.data.status);
      } else if (!res.ok && !res.missing) {
        error.value = t('modelPrices.dismissNotPersisted', { error: res.error });
        return;
      }
    }
    pendingCandidates.value = [];
    selectedCandidateKeys.value = new Set();
  } finally {
    dismissingCandidates.value = false;
  }
}
</script>