<script setup lang="ts">
import type { Article } from '~/types/cms'
import { ApiError } from '~/composables/useApi'

definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

const route = useRoute()
const cms = useCms()
const auth = useAuthStore()

const articleId = computed(() => String(route.params.id))
const canPublish = computed(() => auth.hasPermission('article.publish'))

const article = ref<Article | null>(null)
const loading = ref(true)
const errorMsg = ref<string | null>(null)
const successMsg = ref<string | null>(null)

const form = reactive({
  title: '',
  slug: '',
  excerpt: '',
  content_md: '',
  meta_title: '',
  meta_description: '',
  cover_image_id: null as string | null,
})

const saving = ref(false)
const uploadingCover = ref(false)
const publishing = ref(false)
const archiving = ref(false)
const deleting = ref(false)
const confirmDeleteOpen = ref(false)

const coverUrl = computed(() =>
  form.cover_image_id ? cms.imageUrl(form.cover_image_id) : null,
)

useSeoMeta({ title: () => `Edit — ${article.value?.title || 'Artikel'}` })

async function loadArticle() {
  loading.value = true
  errorMsg.value = null
  try {
    const a = await cms.getAdmin(articleId.value)
    article.value = a
    form.title = a.title
    form.slug = a.slug
    form.excerpt = a.excerpt ?? ''
    form.content_md = a.content_md
    form.meta_title = a.meta_title ?? ''
    form.meta_description = a.meta_description ?? ''
    form.cover_image_id = a.cover_image_id ?? null
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal memuat artikel'
  } finally {
    loading.value = false
  }
}

onMounted(loadArticle)

async function onSave() {
  errorMsg.value = null
  successMsg.value = null
  saving.value = true
  try {
    const updated = await cms.update(articleId.value, {
      title: form.title,
      slug: form.slug,
      excerpt: form.excerpt,
      content_md: form.content_md,
      meta_title: form.meta_title,
      meta_description: form.meta_description,
      cover_image_id: form.cover_image_id,
    })
    article.value = updated
    successMsg.value = 'Perubahan tersimpan.'
    setTimeout(() => (successMsg.value = null), 3000)
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal menyimpan'
  } finally {
    saving.value = false
  }
}

const coverInputRef = ref<HTMLInputElement | null>(null)

async function onCoverChange(ev: Event) {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  uploadingCover.value = true
  errorMsg.value = null
  try {
    const img = await cms.uploadImage(file, { articleId: articleId.value })
    form.cover_image_id = img.id
    // Auto-save cover reference — supaya kalau user close tab, cover tetap ter-link.
    await cms.update(articleId.value, { cover_image_id: img.id })
    successMsg.value = 'Cover berhasil diupload & dikonversi ke WebP.'
    setTimeout(() => (successMsg.value = null), 3000)
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal upload cover'
  } finally {
    uploadingCover.value = false
    if (coverInputRef.value) coverInputRef.value.value = ''
  }
}

async function removeCover() {
  form.cover_image_id = null
  try {
    await cms.update(articleId.value, { cover_image_id: null })
    successMsg.value = 'Cover dihapus dari artikel.'
    setTimeout(() => (successMsg.value = null), 3000)
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal hapus cover'
  }
}

async function onPublish() {
  publishing.value = true
  errorMsg.value = null
  try {
    const updated = await cms.publish(articleId.value)
    article.value = updated
    successMsg.value = 'Artikel dipublish.'
    setTimeout(() => (successMsg.value = null), 3000)
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal publish'
  } finally {
    publishing.value = false
  }
}
async function onArchive() {
  archiving.value = true
  errorMsg.value = null
  try {
    const updated = await cms.archive(articleId.value)
    article.value = updated
    successMsg.value = 'Artikel diarchive.'
    setTimeout(() => (successMsg.value = null), 3000)
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal archive'
  } finally {
    archiving.value = false
  }
}
async function onDelete() {
  deleting.value = true
  errorMsg.value = null
  try {
    await cms.remove(articleId.value)
    confirmDeleteOpen.value = false
    await navigateTo('/admin/artikel')
  } catch (e: unknown) {
    errorMsg.value = e instanceof ApiError ? e.message : 'Gagal hapus'
    deleting.value = false
  }
}

function fmtDate(s: string | null | undefined): string {
  if (!s) return '—'
  try {
    return new Date(s).toLocaleString('id-ID', {
      day: '2-digit', month: 'short', year: 'numeric',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return s
  }
}
</script>

<template>
  <section>
    <AdminPageHeader
      :title="loading ? 'Memuat…' : article?.title || 'Artikel'"
      :breadcrumb="[{ label: 'Artikel', to: '/admin/artikel' }, { label: 'Edit' }]"
    >
      <template #actions>
        <AdminStatusBadge v-if="article" :status="article.status" />
        <button
          v-if="canPublish && article && article.status !== 'published'"
          type="button"
          class="inline-flex items-center gap-2 rounded-md border border-emerald-300 bg-canvas px-3 py-1.5 text-sm font-semibold text-emerald-800 hover:bg-emerald-50 transition-colors disabled:opacity-60"
          :disabled="publishing"
          @click="onPublish"
        >
          Publish
        </button>
        <button
          v-if="canPublish && article && article.status === 'published'"
          type="button"
          class="inline-flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-1.5 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors disabled:opacity-60"
          :disabled="archiving"
          @click="onArchive"
        >
          Archive
        </button>
        <button
          v-if="canPublish && article"
          type="button"
          class="inline-flex items-center gap-2 rounded-md border border-brand-200 bg-canvas px-3 py-1.5 text-sm font-semibold text-brand-700 hover:bg-brand-50 hover:border-brand-300 transition-colors"
          @click="confirmDeleteOpen = true"
        >
          Hapus
        </button>
      </template>
    </AdminPageHeader>

    <AlertMessage v-if="errorMsg" variant="error" :message="errorMsg" class="mb-4" />
    <AlertMessage v-if="successMsg" variant="success" :message="successMsg" class="mb-4" />

    <div v-if="loading" class="rounded-lg border border-hairline bg-canvas p-6 text-sm text-ink-500">
      Memuat artikel…
    </div>

    <form v-else-if="article" class="grid gap-6 lg:grid-cols-3" @submit.prevent="onSave">
      <div class="lg:col-span-2 space-y-5">
        <BaseInput id="title" v-model="form.title" label="Judul" required />

        <div>
          <label class="text-sm font-medium text-ink-900">Slug</label>
          <div class="mt-1 flex items-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm focus-within:border-brand-500 focus-within:ring-brand-500/20 focus-within:ring-2 transition-colors">
            <span class="text-ink-400 font-mono text-xs">/artikel/</span>
            <input
              v-model="form.slug"
              type="text"
              class="flex-1 bg-transparent outline-none font-mono text-xs text-ink-900"
            />
          </div>
        </div>

        <div>
          <label class="text-sm font-medium text-ink-900">Excerpt</label>
          <textarea
            v-model="form.excerpt"
            rows="2"
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          />
        </div>

        <div>
          <label class="text-sm font-medium text-ink-900">Konten (Markdown) <span class="text-brand-500">*</span></label>
          <textarea
            v-model="form.content_md"
            rows="16"
            required
            class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-mono placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
          />
        </div>

        <details class="rounded-lg border border-hairline bg-canvas p-4">
          <summary class="cursor-pointer text-sm font-medium text-ink-900">SEO override</summary>
          <div class="mt-4 space-y-4">
            <BaseInput id="meta_title" v-model="form.meta_title" label="Meta title" placeholder="Default: judul artikel" />
            <div>
              <label class="text-sm font-medium text-ink-900">Meta description</label>
              <textarea
                v-model="form.meta_description"
                rows="2"
                maxlength="320"
                class="mt-1 block w-full rounded-md border border-hairline bg-canvas px-3 py-2 text-sm placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
              />
              <p class="mt-1 text-xs text-ink-500">{{ form.meta_description.length }} / 320 karakter</p>
            </div>
          </div>
        </details>

        <div class="flex items-center gap-3 pt-2">
          <button
            type="submit"
            class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-4 py-2 text-sm font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-alt transition-colors disabled:opacity-60"
            :disabled="saving"
          >
            <span
              v-if="saving"
              class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
              aria-hidden="true"
            />
            Simpan Perubahan
          </button>
          <NuxtLink
            to="/admin/artikel"
            class="text-sm text-ink-500 hover:text-ink-900 transition-colors"
          >
            Kembali
          </NuxtLink>
        </div>
      </div>

      <aside class="space-y-4">
        <div class="rounded-lg border border-hairline bg-canvas p-6">
          <h3 class="text-sm font-semibold text-ink-900">Cover image</h3>
          <p class="mt-1 text-xs text-ink-500 leading-relaxed">
            Format JPG/PNG/WebP. Otomatis dikonversi ke WebP di server (§14).
          </p>
          <div v-if="coverUrl" class="mt-3">
            <img
              :src="coverUrl"
              alt="cover"
              class="w-full rounded-md border border-hairline aspect-video object-cover"
              loading="lazy"
            />
            <button
              type="button"
              class="mt-2 text-xs text-brand-700 hover:text-brand-800 transition-colors"
              @click="removeCover"
            >
              Hapus cover
            </button>
          </div>
          <div v-else class="mt-3 rounded-md border border-dashed border-hairline bg-canvas-alt py-6 text-center text-xs text-ink-500">
            Belum ada cover
          </div>
          <label class="mt-3 flex items-center justify-center gap-2 rounded-md border border-hairline bg-canvas px-3 py-2 text-sm font-medium text-ink-700 hover:bg-canvas-alt hover:border-ink-300 transition-colors cursor-pointer">
            <span
              v-if="uploadingCover"
              class="inline-block h-3 w-3 rounded-full border-2 border-ink-500 border-t-transparent animate-spin"
              aria-hidden="true"
            />
            <span>{{ uploadingCover ? 'Uploading…' : coverUrl ? 'Ganti cover' : 'Upload cover' }}</span>
            <input
              ref="coverInputRef"
              type="file"
              accept="image/jpeg,image/png,image/webp"
              class="hidden"
              @change="onCoverChange"
            />
          </label>
        </div>

        <div class="rounded-lg border border-hairline bg-canvas p-6 text-xs text-ink-700 space-y-2">
          <div class="flex justify-between items-center">
            <span class="text-ink-500">Status</span>
            <AdminStatusBadge :status="article.status" />
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">Published at</span>
            <span>{{ fmtDate(article.published_at) }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">Updated at</span>
            <span>{{ fmtDate(article.updated_at) }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-ink-500">ID</span>
            <span class="font-mono text-[10px] truncate ml-2 text-ink-500">{{ article.id }}</span>
          </div>
        </div>
      </aside>
    </form>

    <AdminConfirmDialog
      v-model:open="confirmDeleteOpen"
      :title="`Hapus artikel “${article?.title || ''}”?`"
      message="Aksi permanen, tidak bisa di-undo."
      confirm-label="Ya, hapus"
      variant="danger"
      :loading="deleting"
      @confirm="onDelete"
    />
  </section>
</template>
