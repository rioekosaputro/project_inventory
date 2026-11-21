<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import NavMenu from '@/components/NavMenu.vue'
import UserPanel from '@/components/UserPanel.vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const route = useRoute()

// Akun yang wajib ganti password hanya melihat alur itu: tanpa menu navigasi.
const showNav = computed(() => !auth.mustChangePassword)

const drawer = ref<HTMLDialogElement | null>(null)
const drawerOpen = ref(false)

function openDrawer(): void {
  drawer.value?.showModal?.()
  drawerOpen.value = true
}

function closeDrawer(): void {
  if (drawer.value?.open) drawer.value.close()
}

// Klik pada backdrop mengenai <dialog> itu sendiri (konten mengisi seluruh panel).
function onDrawerClick(event: MouseEvent): void {
  if (event.target === drawer.value) closeDrawer()
}

watch(() => route.fullPath, closeDrawer)

// Melebar ke layar lebar saat laci terbuka: sidebar tetap muncul, jadi laci ditutup.
let wide: MediaQueryList | undefined
const onWideChange = (e: MediaQueryListEvent): void => {
  if (e.matches) closeDrawer()
}
onMounted(() => {
  wide = window.matchMedia?.('(min-width: 768px)')
  wide?.addEventListener('change', onWideChange)
})
onBeforeUnmount(() => wide?.removeEventListener('change', onWideChange))
</script>

<template>
  <a href="#main" class="skip-link">Lewati ke konten utama</a>

  <div class="min-h-dvh md:flex">
    <!-- Sidebar: hanya layar lebar. Di layar sempit tujuan yang sama ada di laci. -->
    <aside
      v-if="showNav"
      class="sticky top-0 hidden h-dvh w-64 shrink-0 flex-col border-e border-slate-200 bg-white md:flex"
    >
      <p
        class="flex h-14 items-center gap-2 border-b border-slate-200 px-4 text-base font-semibold"
      >
        <i class="pi pi-box text-indigo-700" aria-hidden="true" />
        Rental ERP
      </p>
      <div class="flex-1 overflow-y-auto p-3"><NavMenu /></div>
      <div class="border-t border-slate-200 p-3"><UserPanel /></div>
    </aside>

    <div class="flex min-w-0 flex-1 flex-col">
      <!-- Header hanya dibutuhkan di layar sempit (tombol menu) atau saat sidebar tidak ada. -->
      <header
        class="sticky top-0 z-20 flex h-14 items-center gap-2 border-b border-slate-200 bg-white px-4"
        :class="showNav ? 'md:hidden' : ''"
      >
        <button
          v-if="showNav"
          type="button"
          class="inline-flex min-h-11 min-w-11 items-center justify-center rounded-lg text-slate-800 hover:bg-slate-100 md:hidden"
          aria-label="Buka menu"
          aria-controls="nav-drawer"
          :aria-expanded="drawerOpen"
          @click="openDrawer"
        >
          <i class="pi pi-bars" aria-hidden="true" />
        </button>
        <p class="text-base font-semibold" :class="showNav ? 'md:hidden' : ''">Rental ERP</p>

        <div v-if="!showNav" class="ms-auto"><UserPanel compact /></div>
      </header>

      <main
        id="main"
        tabindex="-1"
        class="mx-auto w-full max-w-[1280px] flex-1 p-4 outline-none md:p-6"
      >
        <slot />
      </main>
    </div>
  </div>

  <!-- Laci navigasi (layar sempit): <dialog> modal — Escape, inert, dan pemulihan fokus ditangani browser. -->
  <dialog
    v-if="showNav"
    id="nav-drawer"
    ref="drawer"
    class="nav-drawer"
    aria-label="Menu navigasi"
    @click="onDrawerClick"
    @close="drawerOpen = false"
  >
    <div class="flex h-full flex-col">
      <div class="flex h-14 items-center justify-between border-b border-slate-200 ps-4 pe-2">
        <p class="text-base font-semibold">Rental ERP</p>
        <button
          type="button"
          class="inline-flex min-h-11 min-w-11 items-center justify-center rounded-lg text-slate-800 hover:bg-slate-100"
          aria-label="Tutup menu"
          autofocus
          @click="closeDrawer"
        >
          <i class="pi pi-times" aria-hidden="true" />
        </button>
      </div>
      <div class="flex-1 overflow-y-auto p-3"><NavMenu /></div>
      <div class="border-t border-slate-200 p-3"><UserPanel /></div>
    </div>
  </dialog>
</template>
