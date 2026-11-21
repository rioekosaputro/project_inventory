// Package clock menyediakan waktu yang bisa di-mock dan zona bisnis Asia/Jakarta (WIB).
// Tanggal bisnis (periode tagihan, tanggal POD, nomor dokumen) WAJIB dihitung lewat sini
// (CLAUDE.md invarian #5); timestamp disimpan UTC (TIMESTAMPTZ).
package clock

import (
	"sync"
	"time"
	_ "time/tzdata" // embed data zona waktu: Windows/Alpine/serverless tanpa tzdata tetap benar
)

// WIB adalah zona waktu bisnis (UTC+7, tanpa DST).
var WIB = mustLoad("Asia/Jakarta")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic("clock: gagal memuat zona " + name + ": " + err.Error())
	}
	return loc
}

// Clock adalah sumber waktu yang bisa diganti saat test.
type Clock interface {
	Now() time.Time
}

// System memakai jam sistem.
type System struct{}

func (System) Now() time.Time { return time.Now() }

// DateWIB mengembalikan tanggal bisnis (YYYY-MM-DD) dari t dalam zona WIB.
func DateWIB(t time.Time) string { return t.In(WIB).Format("2006-01-02") }

// Fake adalah jam manual untuk test. Aman dipakai lintas goroutine.
type Fake struct {
	mu sync.Mutex
	t  time.Time
}

func NewFake(t time.Time) *Fake { return &Fake{t: t} }

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = t
}

func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}
