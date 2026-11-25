//go:build integration

package http_test

import (
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/config"
)

func TestLabels_RackAndSerialBatchPDF(t *testing.T) {
	var capturedHTML string
	gotenberg := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		require.Equal(t, "/forms/chromium/convert/html", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(4<<20))
		f, _, err := r.FormFile("files")
		require.NoError(t, err)
		defer f.Close()
		b, _ := io.ReadAll(f)
		capturedHTML = string(b)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4\n%labels"))
	}))
	defer gotenberg.Close()

	e := newEnv(t, func(c *config.Config) { c.GotenbergURL = gotenberg.URL })
	f := newLPBFixture(t, e)
	e.receive(f, f.whA, f.a1, f.p1, 2, "SN-LBL-1", "SN-LBL-2")

	var s1, s2 int64
	require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT id FROM product_serials WHERE serial_number = 'SN-LBL-1'`).Scan(&s1))
	require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT id FROM product_serials WHERE serial_number = 'SN-LBL-2'`).Scan(&s2))

	// ---- Label Rak: POST /racks/labels/pdf ----
	assert.Equal(t, 403, e.post(f.driver, "/api/v1/racks/labels/pdf", map[string]any{"rack_ids": []int64{f.a1}}).status())
	assert.Equal(t, 422, e.post(f.staffA, "/api/v1/racks/labels/pdf", map[string]any{"rack_ids": []int64{}}).status())
	assert.Equal(t, 404, e.post(f.staffA, "/api/v1/racks/labels/pdf", map[string]any{"rack_ids": []int64{999999}}).status())
	assert.Equal(t, 403, e.post(f.staffA, "/api/v1/racks/labels/pdf", map[string]any{"rack_ids": []int64{f.b1}}).status(), "gudang lain ditolak")

	rRack := e.post(f.staffA, "/api/v1/racks/labels/pdf", map[string]any{"rack_ids": []int64{f.a1, f.a2}})
	require.Equal(t, 200, rRack.status(), rRack.rec.Body.String())
	assert.Equal(t, "application/pdf", rRack.rec.Header().Get("Content-Type"))
	assert.Contains(t, rRack.rec.Header().Get("Content-Disposition"), `inline; filename="label-rak.pdf"`)
	assert.Contains(t, capturedHTML, "A-01")
	assert.Contains(t, capturedHTML, "A-02")
	assert.Contains(t, capturedHTML, "NON-SELLABLE")
	assert.Contains(t, capturedHTML, "data:image/png;base64,")

	// ---- Label Asset Tag Batch: POST /inventory/serials/labels/pdf ----
	assert.Equal(t, 403, e.post(f.driver, "/api/v1/inventory/serials/labels/pdf", map[string]any{"serial_ids": []int64{s1}}).status())
	assert.Equal(t, 422, e.post(f.staffA, "/api/v1/inventory/serials/labels/pdf", map[string]any{"serial_ids": []int64{}}).status())
	assert.Equal(t, 404, e.post(f.staffA, "/api/v1/inventory/serials/labels/pdf", map[string]any{"serial_ids": []int64{999999}}).status())
	assert.Equal(t, 403, e.post(f.staffB, "/api/v1/inventory/serials/labels/pdf", map[string]any{"serial_ids": []int64{s1}}).status(), "unit di gudang lain ditolak")

	rSer := e.post(f.staffA, "/api/v1/inventory/serials/labels/pdf?download=1", map[string]any{"serial_ids": []int64{s1, s2}})
	require.Equal(t, 200, rSer.status(), rSer.rec.Body.String())
	assert.Equal(t, "application/pdf", rSer.rec.Header().Get("Content-Type"))
	assert.Contains(t, rSer.rec.Header().Get("Content-Disposition"), `attachment; filename="label-aset.pdf"`)
	assert.Contains(t, capturedHTML, e.tag("SN-LBL-1"))
	assert.Contains(t, capturedHTML, e.tag("SN-LBL-2"))
	assert.Contains(t, capturedHTML, "SN: SN-LBL-1")
	assert.Contains(t, capturedHTML, "data:image/png;base64,")

	// Gotenberg down → 503.
	gotenberg.Close()
	rDown := e.post(f.staffA, "/api/v1/inventory/serials/labels/pdf", map[string]any{"serial_ids": []int64{s1}})
	assert.Equal(t, 503, rDown.status())
	assert.Equal(t, "PDF_SERVICE_UNAVAILABLE", rDown.code())
}
