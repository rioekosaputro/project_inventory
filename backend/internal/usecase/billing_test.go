package usecase

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPeriodForRunDate(t *testing.T) {
	tests := []struct {
		name       string
		runDate    time.Time
		billingDay int
		wantOK     bool
		wantStart  string
		wantEnd    string
	}{
		{
			name:       "Tanggal 1 Desember menagih 1–30 November",
			runDate:    time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			billingDay: 1,
			wantOK:     true,
			wantStart:  "2026-11-01",
			wantEnd:    "2026-11-30",
		},
		{
			name:       "Tanggal 1 Maret tahun non-kabisat menagih 1–28 Februari",
			runDate:    time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			billingDay: 1,
			wantOK:     true,
			wantStart:  "2026-02-01",
			wantEnd:    "2026-02-28",
		},
		{
			name:       "Tanggal 1 Maret tahun kabisat menagih 1–29 Februari",
			runDate:    time.Date(2028, 3, 1, 0, 0, 0, 0, time.UTC),
			billingDay: 1,
			wantOK:     true,
			wantStart:  "2028-02-01",
			wantEnd:    "2028-02-29",
		},
		{
			name:       "Tanggal 15 Desember menagih 15 Nov – 14 Des",
			runDate:    time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC),
			billingDay: 15,
			wantOK:     true,
			wantStart:  "2026-11-15",
			wantEnd:    "2026-12-14",
		},
		{
			name:       "Bukan hari tagih kontrak",
			runDate:    time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC),
			billingDay: 1,
			wantOK:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps, pe, ok := PeriodForRunDate(tc.runDate, tc.billingDay)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.wantStart, ps.Format("2006-01-02"))
				assert.Equal(t, tc.wantEnd, pe.Format("2006-01-02"))
			}
		})
	}
}
