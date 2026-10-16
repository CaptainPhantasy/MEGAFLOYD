<script setup lang="ts">
import logoUrl from '../Floyd.png'

withDefaults(defineProps<{ size?: number }>(), { size: 56 })
</script>

<template>
  <img
    class="FloydLogo"
    :src="logoUrl"
    :width="size"
    :height="size"
    alt="Floyd"
  />
</template>

<style scoped>
.FloydLogo {
  display: block;
  object-fit: contain;
}
</style>
