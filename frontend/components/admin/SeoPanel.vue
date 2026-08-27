<script setup lang="ts">
/**
 * AdminSeoPanel — analisis SEO on-page gaya Rank Math untuk editor artikel.
 * Field kata kunci & meta title/description dua-arah lewat v-model bernama
 * (`v-model:focus-keyword`, dst) — pola props+emit eksplisit, konsisten
 * dengan komponen lain di codebase ini (mis. `ProductMultiSelect.vue`).
 */
import { CircleCheck, CircleX, Search } from '@lucide/vue'
import { analyzeSeo } from '~/composables/useSeoAnalysis'

const props = defineProps<{
  title: string
  slug: string
  contentMd: string
  coverAltText: string
  focusKeyword: string
  secondaryKeywords: string
  metaTitle: string
  metaDescription: string
}>()

const emit = defineEmits<{
  (e: 'update:focusKeyword' | 'update:secondaryKeywords' | 'update:metaTitle' | 'update:metaDescription', value: string): void
  (e: 'update:score', value: number): void
}>()

const analysis = computed(() => analyzeSeo({
  title: props.title,
  slug: props.slug,
  metaTitle: props.metaTitle,
  metaDescription: props.metaDescription,
  contentMd: props.contentMd,
  focusKeyword: props.focusKeyword,
  secondaryKeywords: props.secondaryKeywords,
  coverAltText: props.coverAltText,
}))

watch(() => analysis.value.score, (score) => emit('update:score', score), { immediate: true })

const scoreTone = computed<'good' | 'ok' | 'bad'>(() => {
  const s = analysis.value.score
  if (s >= 80) return 'good'
  if (s >= 50) return 'ok'
  return 'bad'
})

const scoreBarClass = computed(() => ({
  good: 'bg-emerald-500',
  ok: 'bg-amber-500',
  bad: 'bg-brand-500',
}[scoreTone.value]))

const scoreLabelClass = computed(() => ({
  good: 'text-emerald-700',
  ok: 'text-amber-700',
  bad: 'text-brand-700',
}[scoreTone.value]))

const scoreLabel = computed(() => ({
  good: 'Sangat Baik',
  ok: 'Cukup',
  bad: 'Perlu Perbaikan',
}[scoreTone.value]))

const metaTitleEffectiveLength = computed(() => (props.metaTitle || props.title).length)
const metaTitleLenOk = computed(() => metaTitleEffectiveLength.value >= 40 && metaTitleEffectiveLength.value <= 60)
const metaDescLenOk = computed(() => props.metaDescription.length >= 120 && props.metaDescription.length <= 160)

const serpTitle = computed(() => props.metaTitle || props.title || 'Judul artikel')
const serpUrl = computed(() => `rajakuprinting.com/artikel/${props.slug || '...'}`)
const serpDesc = computed(() => props.metaDescription || 'Meta description akan tampil di sini…')
</script>

<template>
  <div class="rounded-lg border border-hairline bg-canvas p-6">
    <h3 class="flex items-center gap-2 text-sm font-semibold text-ink-900">
      <Search class="h-4 w-4 text-ink-500" :stroke-width="1.5" />
      SEO
    </h3>

    <div class="mt-4">
      <div class="flex items-center justify-between">
        <span class="text-2xl font-sans font-semibold text-ink-950">{{ analysis.score }}</span>
        <span class="text-xs" :class="scoreLabelClass">{{ scoreLabel }}</span>
      </div>
      <div class="mt-2 h-2 overflow-hidden rounded-full bg-canvas-alt">
        <div
          class="h-full rounded-full transition-all duration-200 ease-out"
          :class="scoreBarClass"
          :style="{ width: `${analysis.score}%` }"
        />
      </div>
    </div>

    <div class="mt-5 space-y-4">
      <BaseInput
        id="seo-focus-keyword"
        :model-value="focusKeyword"
        label="Kata kunci utama"
        placeholder="mis. banner outdoor tahan air"
        :max-length="100"
        @update:model-value="emit('update:focusKeyword', $event)"
      />

      <div>
        <BaseInput
          id="seo-secondary-keywords"
          :model-value="secondaryKeywords"
          label="Kata kunci turunan"
          placeholder="pisahkan dengan koma"
          :max-length="300"
          @update:model-value="emit('update:secondaryKeywords', $event)"
        />
        <p class="mt-1 text-xs text-ink-500">Contoh: spanduk outdoor, banner flexi</p>
      </div>

      <div>
        <BaseInput
          id="seo-meta-title"
          :model-value="metaTitle"
          label="Meta title"
          @update:model-value="emit('update:metaTitle', $event)"
        />
        <p class="mt-1 text-xs" :class="metaTitleLenOk ? 'text-emerald-700' : 'text-amber-700'">
          {{ metaTitleEffectiveLength }} / 60 karakter (ideal 40–60)
        </p>
      </div>

      <div>
        <label class="text-sm font-medium text-ink-900">Meta description</label>
        <textarea
          :value="metaDescription"
          rows="3"
          maxlength="320"
          class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          @input="emit('update:metaDescription', ($event.target as HTMLTextAreaElement).value)"
        />
        <p class="mt-1 text-xs" :class="metaDescLenOk ? 'text-emerald-700' : 'text-amber-700'">
          {{ metaDescription.length }} / 160 karakter (ideal 120–160)
        </p>
      </div>

      <div class="space-y-0.5 rounded-md border border-hairline bg-canvas-alt p-3">
        <p class="truncate text-sm text-ink-950">{{ serpTitle }}</p>
        <p class="truncate font-mono text-xs text-ink-500">{{ serpUrl }}</p>
        <p class="line-clamp-2 text-xs text-ink-500">{{ serpDesc }}</p>
      </div>

      <ul class="space-y-1.5 text-xs">
        <li v-for="check in analysis.checks" :key="check.id" class="flex items-start gap-2">
          <CircleCheck v-if="check.passed" class="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-emerald-600" :stroke-width="1.5" />
          <CircleX v-else class="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-brand-500" :stroke-width="1.5" />
          <span :class="check.passed ? 'text-ink-700' : 'text-ink-500'">{{ check.label }}</span>
        </li>
      </ul>
    </div>
  </div>
</template>
