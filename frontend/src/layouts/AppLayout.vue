<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import NavMenu from '@/components/NavMenu.vue'
import UserPanel from '@/components/UserPanel.vue'
import { useAuthStore } from '@/stores/auth'

const SIDEBAR_STORAGE_KEY = 'erp.sidebar.collapsed'

const auth = useAuthStore()
const route = useRoute()

// Akun yang wajib ganti password hanya melihat alur itu: tanpa menu navigasi.
const showNav = computed(() => !auth.mustChangePassword)

function loadSidebarState(): boolean {
  try {
    return window.localStorage.getItem(SIDEBAR_STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

const sidebarCollapsed = ref(loadSidebarState())

function toggleSidebar(): void {
  sidebarCollapsed.value = !sidebarCollapsed.value
  try {
    window.localStorage.setItem(SIDEBAR_STORAGE_KEY, sidebarCollapsed.value ? '1' : '0')
  } catch {
    // abaikan bila localStorage tidak tersedia
  }
}

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

  <div class="min-h-dvh overflow-x-clip md:flex">
    <!-- Sidebar: hanya layar lebar, dapat dibuka/tutup (konten utama dinamis flex-1). -->
    <aside
      v-if="showNav"
      class="sticky top-0 hidden h-dvh shrink-0 flex-col border-e border-slate-200 bg-white transition-[width] duration-200 ease-in-out md:flex"
      :class="sidebarCollapsed ? 'w-[72px]' : 'w-60'"
    >
      <div
        class="flex h-14 items-center border-b border-slate-200"
        :class="sidebarCollapsed ? 'justify-center px-2' : 'justify-between ps-4 pe-2'"
      >
        <p
          v-if="!sidebarCollapsed"
          class="flex items-center gap-2 truncate text-base font-semibold text-slate-900"
        >
          <i class="pi pi-box text-indigo-700" aria-hidden="true" />
          <span>Rental ERP</span>
        </p>
        <button
          type="button"
          class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-600 hover:bg-slate-100 hover:text-slate-900"
          :aria-label="sidebarCollapsed ? 'Buka sidebar' : 'Ciutkan sidebar'"
          :title="sidebarCollapsed ? 'Buka sidebar' : 'Ciutkan sidebar'"
          :aria-expanded="!sidebarCollapsed"
          @click="toggleSidebar"
        >
          <i
            :class="sidebarCollapsed ? 'pi pi-angle-double-right' : 'pi pi-angle-double-left'"
            aria-hidden="true"
          />
        </button>
      </div>
      <div class="flex-1 overflow-x-hidden overflow-y-auto p-2.5">
        <NavMenu :collapsed="sidebarCollapsed" />
      </div>
      <div class="border-t border-slate-200 p-2.5">
        <UserPanel :collapsed="sidebarCollapsed" />
      </div>
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
        class="w-full min-w-0 flex-1 bg-slate-50/50 p-4 outline-none md:px-6 md:py-5"
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
    <div class="flex h-full min-h-0 flex-1 flex-col">
      <div class="flex h-14 shrink-0 items-center justify-between border-b border-slate-200 ps-4 pe-2">
        <p class="flex items-center gap-2 text-base font-semibold text-slate-900">
          <i class="pi pi-box text-indigo-700" aria-hidden="true" />
          <span>Rental ERP</span>
        </p>
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
      <div class="min-h-0 flex-1 overflow-y-auto p-3"><NavMenu /></div>
      <div class="shrink-0 border-t border-slate-200 bg-white p-3"><UserPanel /></div>
    </div>
  </dialog>
</template>
