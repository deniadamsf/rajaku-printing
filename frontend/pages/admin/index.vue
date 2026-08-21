<script setup lang="ts">
definePageMeta({
  middleware: ['staff-only'],
  layout: 'admin',
})

useSeoMeta({
  title: 'Dashboard — Admin',
})

const auth = useAuthStore()
const nav = useAdminNav()

const groupLabels: Record<'operasional' | 'konten' | 'kelola', string> = {
  operasional: 'Operasional',
  konten: 'Konten',
  kelola: 'Kelola',
}
</script>

<template>
  <section>
    <AdminPageHeader
      title="Dashboard"
      :subtitle="`Halo, ${auth.user?.name || 'Staff'}. Pilih modul yang mau dikerjakan.`"
    />

    <div
      v-if="nav.visibleItems.value.length === 0"
      class="rounded-lg border border-gold-200 bg-gold-50 p-4 text-sm text-gold-900"
    >
      Akun kamu belum punya permission ke modul admin apa pun. Hubungi super admin
      untuk assign role.
    </div>

    <div
      v-for="(items, group) in nav.grouped.value"
      v-else
      :key="group"
      :class="items.length === 0 ? 'hidden' : 'mb-10'"
    >
      <p class="text-[10px] font-medium uppercase tracking-[0.14em] text-ink-500 mb-3">
        {{ groupLabels[group as 'operasional' | 'konten' | 'kelola'] }}
      </p>
      <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <NuxtLink
          v-for="item in items"
          :key="item.to"
          :to="item.to"
          class="group flex items-start gap-4 rounded-lg border border-hairline bg-canvas p-5 transition-colors hover:border-ink-300"
        >
          <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-canvas-alt text-ink-700 transition-colors group-hover:bg-brand-500 group-hover:text-canvas">
            <component :is="item.icon" class="h-5 w-5" :stroke-width="1.5" />
          </span>
          <div class="flex-1 min-w-0">
            <h3 class="text-sm font-semibold text-ink-950 transition-colors group-hover:text-brand-500">
              {{ item.label }}
            </h3>
            <p class="mt-0.5 text-xs text-ink-500 leading-relaxed">{{ item.description }}</p>
          </div>
        </NuxtLink>
      </div>
    </div>

    <details class="mt-12 rounded-lg border border-hairline bg-canvas p-4 text-xs text-ink-500">
      <summary class="cursor-pointer font-medium text-ink-700">Debug: permission aktif</summary>
      <ul class="mt-3 grid gap-1 sm:grid-cols-2 md:grid-cols-3">
        <li
          v-for="p in auth.permissions"
          :key="p"
          class="rounded bg-canvas-alt px-2 py-1 font-mono text-[10px] text-ink-700"
        >
          {{ p }}
        </li>
      </ul>
    </details>
  </section>
</template>
