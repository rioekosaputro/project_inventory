package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWIB_IsUTCPlus7(t *testing.T) {
	_, off := time.Date(2026, 11, 21, 12, 0, 0, 0, WIB).Zone()
	assert.Equal(t, 7*3600, off)
}

// Transaksi pukul 23:30 UTC sudah hari berikutnya di WIB: salah zona = salah periode tagihan.
func TestDateWIB_CrossesMidnightRelativeToUTC(t *testing.T) {
	utc := time.Date(2026, 11, 30, 23, 30, 0, 0, time.UTC)
	assert.Equal(t, "2026-12-01", DateWIB(utc))

	assert.Equal(t, "2026-11-30", DateWIB(time.Date(2026, 11, 30, 16, 59, 0, 0, time.UTC)))
}

func TestFake(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := NewFake(start)
	assert.Equal(t, start, f.Now())

	f.Advance(90 * time.Minute)
	assert.Equal(t, start.Add(90*time.Minute), f.Now())

	f.Set(start)
	assert.Equal(t, start, f.Now())
}
