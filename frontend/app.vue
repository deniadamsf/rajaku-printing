<script setup lang="ts">
useHead({
  titleTemplate: (title) => (title ? `${title} — Rajaku Printing` : 'Rajaku Printing'),
})
</script>

<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
</template>
