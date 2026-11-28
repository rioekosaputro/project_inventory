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
	"rental-inventory-api/internal/domain"
)

func TestLPB_PDF_And_PublicVerify(t *testing.T) {
	var capturedHTML string
	gotenberg := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		require.Equal(t, "/forms/chromium/convert/html", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(4<<20))
		f, fh, err := r.FormFile("files")
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		assert.Equal(t, "index.html", fh.Filename)
		b, _ := io.ReadAll(f)
		capturedHTML = string(b)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4\n%surat-jalan"))
	}))
	defer gotenberg.Close()

	e := newEnv(t, func(c *config.Config) {
		c.GotenbergURL = gotenberg.URL
		c.AppBaseURL = "https://erp.contoh.id"
	})
	f := newLPBFixture(t, e)

	// 1. Draft & Pending tidak boleh dicetak (422 LPB_NOT_APPROVED).
	draftID := idOf(e.mustLPB(f.staffA, f.purchase(lpbItem(f.p2, f.a1, 2))))
	rDraft := e.get(f.staffA, lpbPath(draftID, "/pdf"))
	assert.Equal(t, 422, rDraft.status())
	assert.Equal(t, "LPB_NOT_APPROVED", rDraft.code())

	pendingID, _ := e.pending(f.staffA, f.purchase(lpbItem(f.p1, f.a1, 1, map[string]any{"unit_cost": "1250000"}), lpbItem(f.p2, f.a1, 3)), map[int][]string{0: {"SN-PDF-1"}})
	rPending := e.get(f.staffA, lpbPath(pendingID, "/pdf"))
	assert.Equal(t, 422, rPending.status())
	assert.Equal(t, "LPB_NOT_APPROVED", rPending.code())

	// Approve LPB → siap dicetak & diverifikasi.
	ap := e.approve(f.headA, pendingID, nextKey())
	require.Equal(t, 200, ap.status(), ap.rec.Body.String())
	lpbNumber := ap.str("lpb_number")

	// 2. RBAC & Scoping untuk /pdf.
	assert.Equal(t, 401, e.call("GET", lpbPath(pendingID, "/pdf"), nil).status())
	assert.Equal(t, 403, e.get(f.tech, lpbPath(pendingID, "/pdf")).status(), "TECHNICIAN tidak punya lpb.print")
	assert.Equal(t, 403, e.get(f.driver, lpbPath(pendingID, "/pdf")).status(), "DRIVER tanpa penugasan ditolak")
	_, headB := e.userToken(domain.RoleHeadWarehouse, &f.whB)
	assert.Equal(t, 403, e.get(headB, lpbPath(pendingID, "/pdf")).status(), "gudang lain ditolak")

	// DRIVER yang ditugaskan boleh mencetak.
	driverID, driverAssignedTok := e.userToken(domain.RoleDriver, nil)
	e.exec(`UPDATE lpb_documents SET driver_id = $2 WHERE id = $1`, pendingID, driverID)
	rDriver := e.get(driverAssignedTok, lpbPath(pendingID, "/pdf"))
	assert.Equal(t, 200, rDriver.status())

	// 3. Cetak inline & download=1 oleh staf gudang.
	rInline := e.get(f.staffA, lpbPath(pendingID, "/pdf"))
	require.Equal(t, 200, rInline.status())
	assert.Equal(t, "application/pdf", rInline.rec.Header().Get("Content-Type"))
	assert.Contains(t, rInline.rec.Header().Get("Content-Disposition"), "inline; filename=")
	assert.Equal(t, "%PDF-1.4\n%surat-jalan", rInline.rec.Body.String())

	// HTML yang dikirim ke Gotenberg memuat nomor LPB, SKU, SN, dan QR data URI.
	assert.Contains(t, capturedHTML, lpbNumber)
	assert.Contains(t, capturedHTML, "PRN-1")
	assert.Contains(t, capturedHTML, "SN-PDF-1")
	assert.Contains(t, capturedHTML, "data:image/png;base64,")

	rDownload := e.get(f.mgmt, lpbPath(pendingID, "/pdf?download=1"))
	require.Equal(t, 200, rDownload.status())
	assert.Contains(t, rDownload.rec.Header().Get("Content-Disposition"), "attachment; filename=")

	// 4. Verifikasi publik via QR token (tanpa autentikasi).
	var token string
	require.NoError(t, e.pool.QueryRow(testCtx(), `SELECT verify_token FROM lpb_documents WHERE id = $1`, pendingID).Scan(&token))
	require.NotEmpty(t, token)

	pub := e.call("GET", "/api/v1/public/lpb/verify/"+token, nil)
	require.Equal(t, 200, pub.status(), pub.rec.Body.String())
	data := pub.data()
	assert.Equal(t, lpbNumber, data["lpb_number"])
	assert.Equal(t, "INBOUND", data["type"])
	assert.Equal(t, "PURCHASE", data["sub_type"])
	assert.Equal(t, "APPROVED", data["status"])
	assert.Equal(t, "WH-A", data["warehouse_code"])
	assert.Equal(t, "PT Maju Jaya", data["supplier_name"])
	assert.EqualValues(t, 2, data["item_count"])
	assert.EqualValues(t, 4, data["total_qty"])
	assert.NotEmpty(t, data["approved_by_name"])
	assert.NotNil(t, data["approved_at"])
	// Pastikan data sensitif tidak bocor di endpoint publik.
	bodyStr := pub.rec.Body.String()
	assert.NotContains(t, bodyStr, "SN-PDF-1")
	assert.NotContains(t, bodyStr, "1250000")
	assert.NotContains(t, data, "id")

	// Token tidak valid / terlalu pendek → 404.
	assert.Equal(t, 404, e.call("GET", "/api/v1/public/lpb/verify/short", nil).status())
	assert.Equal(t, 404, e.call("GET", "/api/v1/public/lpb/verify/token-tidak-terdaftar-0123456789", nil).status())

	// 5. Gotenberg down → 503 PDF_SERVICE_UNAVAILABLE.
	gotenberg.Close()
	rDown := e.get(f.staffA, lpbPath(pendingID, "/pdf"))
	assert.Equal(t, 503, rDown.status())
	assert.Equal(t, "PDF_SERVICE_UNAVAILABLE", rDown.code())
}
