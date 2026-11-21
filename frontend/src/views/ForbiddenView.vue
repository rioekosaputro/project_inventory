<template>
  <section
    class="mx-auto mt-8 max-w-md rounded-xl border border-slate-200 bg-white p-8 text-center shadow-sm"
  >
    <p class="text-sm font-semibold text-indigo-700">Kode 403</p>
    <h1 class="mt-2 text-xl font-semibold text-slate-900">Akses ditolak</h1>
    <p class="mt-2 text-sm text-slate-600">
      Peran Anda tidak memiliki izin untuk membuka halaman ini. Hubungi administrator bila Anda
      merasa seharusnya punya akses.
    </p>
    <RouterLink
      :to="{ name: 'home' }"
      class="mt-6 inline-flex min-h-11 items-center rounded-lg bg-indigo-700 px-5 text-sm font-semibold text-white hover:bg-indigo-800"
    >
      Kembali ke Beranda
    </RouterLink>
  </section>
</template>
