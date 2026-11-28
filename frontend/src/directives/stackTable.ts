import type { Directive } from 'vue'

/**
 * `v-stack-table`: di layar HP tabel tampil sebagai tumpukan kartu (lihat `.stack-table` di
 * main.css). Judul kolom dari <thead> disalin ke `data-label` tiap sel, sehingga template tabel
 * tidak perlu menuliskannya dua kali.
 */
function label(table: HTMLTableElement): void {
  const headers = Array.from(table.tHead?.rows[0]?.cells ?? [], (th) =>
    (th.textContent ?? '').trim(),
  )
  for (const body of Array.from(table.tBodies)) {
    for (const row of Array.from(body.rows)) {
      let column = 0
      for (const cell of Array.from(row.cells)) {
        const text = headers[column]
        // Sel gabungan (pesan kosong, rincian baris) tampil selebar kartu tanpa label.
        if (text && cell.colSpan === 1) cell.setAttribute('data-label', text)
        else cell.removeAttribute('data-label')
        column += cell.colSpan
      }
    }
  }
}

export const vStackTable: Directive<HTMLTableElement> = {
  mounted(table) {
    table.classList.add('stack-table')
    label(table)
  },
  updated(table) {
    table.classList.add('stack-table')
    label(table)
  },
}
