<script setup lang="ts">
import { ApiError } from '~/composables/useApi'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({ title: 'Artikel Baru — Admin' })

const cms = useCms()

const form = reactive({
  title: '',
  slug: '',
  excerpt: '',
  content_md: '',
  meta_title: '',
  meta_description: '',
})

const submitting = ref(false)
const errorMsg = ref<string | null>(null)

// Preview slug — simple client-side (backend re-generate final).
const slugPreview = computed(() => {
  if (form.slug.trim()) return form.slug.trim()
  return form.title
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 200) || '(auto)'
})

async function onSubmit() {
  errorMsg.value = null
  submitting.value = true
  try {
    const created = await cms.create({
      title: form.title,
      slug: form.slug || undefined,
      excerpt: form.excerpt || undefined,
      content_md: form.content_md,
      meta_title: form.meta_title || undefined,
      meta_description: form.meta_description || undefined,
    })
    await navigateTo(`/admin/artikel/${created.id}`)
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal membuat artikel'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Artikel Baru"
      subtitle="Simpan dulu sebagai draft; upload cover & publish di halaman edit."
      :breadcrumb="[{ label: 'Artikel', to: '/admin/artikel' }, { label: 'Baru' }]"
    />

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />

    <form class="max-w-3xl space-y-5" @submit.prevent="onSubmit">
      <BaseInput
        id="title"
        v-model="form.title"
        label="Judul"
        required
        placeholder="Contoh: 5 Tips Pilih Bahan Banner Outdoor"
      />

      <div>
        <label class="text-sm font-medium text-ink-900">Slug (opsional)</label>
        <div class="mt-1 flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm focus-within:border-brand-500 focus-within:ring-brand-500/20 focus-within:ring-2 transition-colors">
          <span class="text-ink-400 font-mono text-xs">/artikel/</span>
          <input
            v-model="form.slug"
            type="text"
            placeholder="auto-generate dari judul"
            class="flex-1 bg-transparent outline-none font-mono text-xs text-ink-900 placeholder-ink-400"
          >
        </div>
        <p class="mt-1 text-xs text-ink-500">
          Preview slug: <code class="rounded bg-canvas-alt px-1.5 py-0.5 font-mono text-[10px] text-ink-700">{{ slugPreview }}</code>
        </p>
      </div>

      <div>
        <label class="text-sm font-medium text-ink-900">
          Excerpt
          <span class="text-ink-400 font-normal">(ringkasan pendek, dipakai di list & fallback meta description)</span>
        </label>
        <textarea
          v-model="form.excerpt"
          rows="2"
          class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          placeholder="1-2 kalimat ringkasan"
        />
      </div>

      <div>
        <label class="text-sm font-medium text-ink-900">
          Konten (Markdown)
          <span class="text-brand-500">*</span>
        </label>
        <textarea
          v-model="form.content_md"
          rows="10"
          required
          class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-mono placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          placeholder="# Judul&#10;&#10;Body markdown…"
        />
      </div>

      <details class="rounded-lg border border-hairline bg-canvas p-4">
        <summary class="cursor-pointer text-sm font-medium text-ink-900">SEO override (opsional)</summary>
        <div class="mt-4 space-y-4">
          <BaseInput
            id="meta_title"
            v-model="form.meta_title"
            label="Meta title (override <title>)"
            placeholder="Default: judul artikel"
          />
          <div>
            <label class="text-sm font-medium text-ink-900">Meta description</label>
            <textarea
              v-model="form.meta_description"
              rows="2"
              maxlength="320"
              class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              placeholder="Default: excerpt"
            />
            <p class="mt-1 text-xs text-ink-500">{{ form.meta_description.length }} / 320 karakter</p>
          </div>
        </div>
      </details>

      <div class="flex items-center gap-3 pt-2">
        <button
          type="submit"
          class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt transition-colors disabled:opacity-60"
          :disabled="submitting"
        >
          <span
            v-if="submitting"
            class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
            aria-hidden="true"
          />
          Simpan Draft
        </button>
        <NuxtLink
          to="/admin/artikel"
          class="text-sm text-ink-500 hover:text-ink-900 transition-colors"
        >
          Batal
        </NuxtLink>
      </div>
    </form>
  </section>
</template>
