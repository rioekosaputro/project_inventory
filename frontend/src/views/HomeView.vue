<script setup lang="ts">
// Pola halaman: Dashboard / Overview (sementara). Isi sebenarnya per peran menyusul (T1.14 dst.).
import { computed } from 'vue'
import { roleLabel } from '@/lib/labels'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()

const scope = computed(() =>
  auth.user?.warehouse_id == null ? 'Semua gudang' : `Gudang #${auth.user.warehouse_id}`,
)
</script>

<template>
  <section v-if="auth.user">
    <h1 class="text-xl font-semibold text-slate-900">Selamat datang, {{ auth.user.name }}</h1>
    <p class="mt-1 text-sm text-slate-600">Ringkasan akses akun Anda.</p>

    <dl class="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <div class="rounded-xl border border-slate-200 bg-white p-4">
        <dt class="text-sm text-slate-600">Peran</dt>
        <dd class="mt-1 text-lg font-semibold text-slate-900">{{ roleLabel(auth.user.role) }}</dd>
      </div>
      <div class="rounded-xl border border-slate-200 bg-white p-4">
        <dt class="text-sm text-slate-600">Cakupan gudang</dt>
        <dd class="mt-1 text-lg font-semibold text-slate-900">{{ scope }}</dd>
      </div>
      <div class="rounded-xl border border-slate-200 bg-white p-4">
        <dt class="text-sm text-slate-600">Hak akses aktif</dt>
        <dd class="mt-1 text-lg font-semibold text-slate-900">{{ auth.permissions.length }}</dd>
      </div>
    </dl>

    <p class="mt-6 text-sm text-slate-600">
      Menu di sebelah kiri menyesuaikan peran Anda dan bertambah seiring modul selesai dibangun.
    </p>
  </section>
</template>
