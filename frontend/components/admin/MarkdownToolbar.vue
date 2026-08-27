<script setup lang="ts">
/**
 * AdminMarkdownToolbar — toolbar format markdown + textarea konten artikel.
 * Memegang ref textarea sendiri supaya bisa memanipulasi seleksi/kursor
 * langsung (heading toggle, wrap bold/italic, list prefix, sisip gambar).
 *
 * Upload gambar dipakai lewat popover kecil (bukan window.prompt — §26.6),
 * hasilnya disisipkan sebagai `![alt](url)` di posisi kursor terakhir.
 */
import {
  Bold,
  Heading1,
  Heading2,
  Heading3,
  Heading4,
  Image as ImageIcon,
  Italic,
  Link2,
  List,
  ListOrdered,
  Quote,
} from '@lucide/vue'
import { ApiError } from '~/composables/useApi'

const props = withDefaults(defineProps<{
  modelValue: string
  rows?: number
  articleId?: string | null
}>(), {
  rows: 16,
  articleId: null,
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const cms = useCms()
const textareaRef = ref<HTMLTextAreaElement | null>(null)

function emitAndRestore(newValue: string, selStart: number, selEnd: number) {
  emit('update:modelValue', newValue)
  nextTick(() => {
    const ta = textareaRef.value
    if (!ta) return
    ta.focus()
    ta.setSelectionRange(selStart, selEnd)
  })
}

function currentLineBounds(value: string, pos: number): { lineStart: number; lineEnd: number } {
  const lineStart = value.lastIndexOf('\n', pos - 1) + 1
  let lineEnd = value.indexOf('\n', pos)
  if (lineEnd === -1) lineEnd = value.length
  return { lineStart, lineEnd }
}

/**
 * Toggle prefix baris (heading/list/ordered/quote). `multiLevel` dipakai
 * khusus heading — ada 4 level, jadi "sudah ada prefix tapi beda level"
 * berarti diganti, bukan dihapus. Prefix tunggal (list/ordered/quote) cukup
 * anggap "ada match apa pun" berarti sudah aktif → toggle off.
 */
function toggleLinePrefix(pattern: RegExp, target: string, multiLevel = false) {
  const ta = textareaRef.value
  if (!ta) return
  const value = props.modelValue
  const pos = ta.selectionStart
  const { lineStart, lineEnd } = currentLineBounds(value, pos)
  const line = value.slice(lineStart, lineEnd)
  const match = line.match(pattern)

  let newLine: string
  if (match) {
    newLine = (!multiLevel || match[0] === target) ? line.slice(match[0].length) : target + line.slice(match[0].length)
  } else {
    newLine = target + line
  }

  const newValue = value.slice(0, lineStart) + newLine + value.slice(lineEnd)
  const delta = newLine.length - line.length
  const newPos = Math.min(Math.max(pos + delta, lineStart), lineStart + newLine.length)
  emitAndRestore(newValue, newPos, newPos)
}

function toggleHeading(level: number) {
  toggleLinePrefix(/^#{1,4}\s+/, `${'#'.repeat(level)} `, true)
}
function toggleList() {
  toggleLinePrefix(/^-\s+/, '- ')
}
function toggleOrderedList() {
  toggleLinePrefix(/^\d+\.\s+/, '1. ')
}
function toggleQuote() {
  toggleLinePrefix(/^>\s+/, '> ')
}

function wrapSelection(marker: string, placeholder: string) {
  const ta = textareaRef.value
  if (!ta) return
  const value = props.modelValue
  const start = ta.selectionStart
  const end = ta.selectionEnd
  const selected = value.slice(start, end)

  let selStart: number
  let selEnd: number
  let newValue: string
  if (selected) {
    newValue = value.slice(0, start) + marker + selected + marker + value.slice(end)
    selStart = start + marker.length
    selEnd = selStart + selected.length
  } else {
    newValue = value.slice(0, start) + marker + placeholder + marker + value.slice(end)
    selStart = start + marker.length
    selEnd = selStart + placeholder.length
  }
  emitAndRestore(newValue, selStart, selEnd)
}

function insertLink() {
  const ta = textareaRef.value
  if (!ta) return
  const value = props.modelValue
  const start = ta.selectionStart
  const end = ta.selectionEnd
  const selected = value.slice(start, end)
  const label = selected || 'teks'
  const inserted = `[${label}](url)`
  const newValue = value.slice(0, start) + inserted + value.slice(end)

  let selStart: number
  let selEnd: number
  if (selected) {
    // Selection sudah jadi label — arahkan fokus ke placeholder "url".
    selStart = start + 1 + label.length + 2
    selEnd = selStart + 3
  } else {
    selStart = start + 1
    selEnd = selStart + label.length
  }
  emitAndRestore(newValue, selStart, selEnd)
}

function insertAtCursor(text: string) {
  const ta = textareaRef.value
  if (!ta) return
  const value = props.modelValue
  const start = ta.selectionStart
  const end = ta.selectionEnd
  const newValue = value.slice(0, start) + text + value.slice(end)
  const newPos = start + text.length
  emitAndRestore(newValue, newPos, newPos)
}

// ---- Popover sisip gambar ----
const showImagePopover = ref(false)
const imageFile = ref<File | null>(null)
const imageAlt = ref('')
const uploadingImage = ref(false)
const imageError = ref<string | null>(null)

function openImagePopover() {
  imageFile.value = null
  imageAlt.value = ''
  imageError.value = null
  showImagePopover.value = !showImagePopover.value
}
function closeImagePopover() {
  showImagePopover.value = false
}
function onImageFileChange(ev: Event) {
  const input = ev.target as HTMLInputElement
  imageFile.value = input.files?.[0] ?? null
}
async function onInsertImage() {
  if (!imageFile.value) {
    imageError.value = 'Pilih file gambar dulu.'
    return
  }
  uploadingImage.value = true
  imageError.value = null
  try {
    const img = await cms.uploadImage(imageFile.value, {
      articleId: props.articleId ?? undefined,
      altText: imageAlt.value || undefined,
    })
    insertAtCursor(`![${imageAlt.value}](${cms.imageUrl(img.id)})\n`)
    closeImagePopover()
  } catch (e: unknown) {
    imageError.value = e instanceof ApiError ? e.message : 'Gagal upload gambar'
  } finally {
    uploadingImage.value = false
  }
}

const btnClass = 'inline-flex h-7 w-7 items-center justify-center rounded-md border border-hairline text-ink-600 hover:bg-canvas hover:text-ink-900 active:bg-ink-950 active:text-canvas transition-colors duration-150 ease-out focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-1 focus-visible:ring-offset-canvas-alt'
</script>

<template>
  <div>
    <div class="flex flex-wrap items-center gap-1 rounded-t-md border border-hairline border-b-0 bg-canvas-alt px-2 py-1.5">
      <button type="button" :class="btnClass" title="Heading 1" @click="toggleHeading(1)">
        <Heading1 class="h-4 w-4" :stroke-width="1.5" />
      </button>
      <button type="button" :class="btnClass" title="Heading 2" @click="toggleHeading(2)">
        <Heading2 class="h-4 w-4" :stroke-width="1.5" />
      </button>
      <button type="button" :class="btnClass" title="Heading 3" @click="toggleHeading(3)">
        <Heading3 class="h-4 w-4" :stroke-width="1.5" />
      </button>
      <button type="button" :class="btnClass" title="Heading 4" @click="toggleHeading(4)">
        <Heading4 class="h-4 w-4" :stroke-width="1.5" />
      </button>

      <span class="mx-1 h-5 w-px bg-ink-200" aria-hidden="true" />

      <button type="button" :class="btnClass" title="Bold" @click="wrapSelection('**', 'teks')">
        <Bold class="h-4 w-4" :stroke-width="1.5" />
      </button>
      <button type="button" :class="btnClass" title="Italic" @click="wrapSelection('_', 'teks')">
        <Italic class="h-4 w-4" :stroke-width="1.5" />
      </button>

      <span class="mx-1 h-5 w-px bg-ink-200" aria-hidden="true" />

      <button type="button" :class="btnClass" title="Bullet list" @click="toggleList">
        <List class="h-4 w-4" :stroke-width="1.5" />
      </button>
      <button type="button" :class="btnClass" title="Numbered list" @click="toggleOrderedList">
        <ListOrdered class="h-4 w-4" :stroke-width="1.5" />
      </button>
      <button type="button" :class="btnClass" title="Quote" @click="toggleQuote">
        <Quote class="h-4 w-4" :stroke-width="1.5" />
      </button>
      <button type="button" :class="btnClass" title="Link" @click="insertLink">
        <Link2 class="h-4 w-4" :stroke-width="1.5" />
      </button>

      <span class="mx-1 h-5 w-px bg-ink-200" aria-hidden="true" />

      <div class="relative">
        <button
          type="button"
          :class="[btnClass, showImagePopover ? 'bg-ink-950 text-canvas' : '']"
          title="Sisipkan gambar"
          @click="openImagePopover"
        >
          <ImageIcon class="h-4 w-4" :stroke-width="1.5" />
        </button>

        <div
          v-if="showImagePopover"
          class="absolute left-0 top-full z-20 mt-1 w-72 space-y-3 rounded-lg border border-hairline bg-canvas p-4 shadow-lg ring-1 ring-black/5"
        >
          <div>
            <label class="text-xs font-medium text-ink-900">File gambar</label>
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp"
              class="mt-1 block w-full text-xs text-ink-700 file:mr-2 file:rounded-md file:border file:border-hairline file:bg-canvas-alt file:px-2 file:py-1 file:text-xs file:text-ink-700 hover:file:bg-canvas"
              @change="onImageFileChange"
            >
          </div>
          <BaseInput id="md-image-alt" v-model="imageAlt" label="Alt text" placeholder="Deskripsi gambar" :max-length="255" />
          <p v-if="imageError" class="text-xs text-brand-500">{{ imageError }}</p>
          <div class="flex items-center justify-end gap-2 pt-1">
            <button
              type="button"
              class="rounded-md px-2 py-1 text-xs text-ink-500 hover:text-ink-900 transition-colors"
              @click="closeImagePopover"
            >
              Batal
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-md bg-brand-500 px-3 py-1.5 text-xs font-semibold text-canvas hover:bg-brand-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-500/40 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas transition-colors disabled:opacity-60"
              :disabled="uploadingImage"
              @click="onInsertImage"
            >
              <span
                v-if="uploadingImage"
                class="inline-block h-3 w-3 rounded-full border-2 border-canvas/70 border-t-transparent animate-spin"
                aria-hidden="true"
              />
              Sisipkan
            </button>
          </div>
        </div>
      </div>
    </div>

    <textarea
      ref="textareaRef"
      :value="modelValue"
      :rows="rows"
      class="mt-1 block w-full rounded-b-md border border-t-0 border-hairline bg-canvas px-3 py-2 text-sm font-mono placeholder-ink-400 text-ink-900 focus:border-brand-500 focus:ring-brand-500/20 focus:ring-2 focus:outline-none transition-colors"
      @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
    />
  </div>
</template>
