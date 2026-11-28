//go:build integration

package http_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
	"rental-inventory-api/pkg/storage"
)

// TestSecurity_IDOR_MaliciousUploads_And_Headers memverifikasi checklist keamanan Bagian 10 (T4.1):
//   - IDOR: Driver A tidak bisa mengakses / mengubah checkpoint, POD, gagal kirim, atau PDF LPB milik Driver B (403).
//   - Upload berbahaya: HTML/script disamarkan sebagai .jpg, script teks disamarkan sebagai .png, dan file > 5 MB ditolak (422);
//     filename path traversal disanitasi ke key UUID server.
//   - Signed URL palsu ditolak 403; header keamanan standar selalu terpasang.
func TestSecurity_IDOR_MaliciousUploads_And_Headers(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)
	biz := e.tokenFor(domain.RoleBusinessAdmin, nil)
	driverA := e.tokenFor(domain.RoleDriver, nil)
	driverB := e.tokenFor(domain.RoleDriver, nil)

	var driverBID int64
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT id FROM users WHERE role = 'DRIVER' ORDER BY id DESC LIMIT 1`).Scan(&driverBID))

	cid, sid := seedCustomerWithSite(t, e, biz, "CUST-SEC-01")
	ctrID := seedActiveContract(t, e, biz, cid, sid, f.p1)
	e.receive(f, f.whA, f.a1, f.p1, 1, "SN-SEC-001")
	var serialID int64
	require.NoError(t, e.pool.QueryRow(t.Context(),
		`SELECT id FROM product_serials WHERE serial_number = 'SN-SEC-001'`).Scan(&serialID))

	// Buat SO -> DO -> Reserve -> LPB RENTAL_DELIVERY -> Submit -> Approve, lalu tugaskan ke Driver B.
	soRes := e.post(biz, "/api/v1/sales-orders", map[string]any{
		"order_type":       "NEW_DELIVERY",
		"customer_id":      cid,
		"customer_site_id": sid,
		"contract_id":      ctrID,
		"items":            []map[string]any{{"product_id": f.p1, "qty": 1}},
	})
	require.Equal(t, 201, soRes.status())
	soID := int64(soRes.data()["id"].(float64))
	require.Equal(t, 200, e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/confirm", soID), nil).status())

	doRes := e.post(biz, fmt.Sprintf("/api/v1/sales-orders/%d/delivery-orders", soID), map[string]any{"warehouse_id": f.whA})
	require.Equal(t, 201, doRes.status())
	doID := int64(doRes.list()[0]["id"].(float64))
	doItemID := int64(e.get(biz, fmt.Sprintf("/api/v1/delivery-orders/%d", doID)).data()["items"].([]any)[0].(map[string]any)["id"].(float64))
	require.Equal(t, 200, e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/reserve", doID), map[string]any{
		"items": []map[string]any{{"do_item_id": doItemID, "rack_id": f.a1, "qty": 1, "serial_ids": []int64{serialID}}},
	}).status())

	lpbRes := e.post(f.staffA, fmt.Sprintf("/api/v1/delivery-orders/%d/lpb", doID), nil)
	require.Equal(t, 201, lpbRes.status())
	lpbID := int64(lpbRes.data()["id"].(float64))
	require.Equal(t, 200, e.post(f.staffA, fmt.Sprintf("/api/v1/lpb/%d/submit", lpbID), nil).status())
	require.Equal(t, 200, e.approve(f.headA, lpbID, "sec-lpb-1").status())

	// Tugaskan LPB ke Driver B.
	_, err := e.pool.Exec(t.Context(), `UPDATE lpb_documents SET driver_id = $2 WHERE id = $1`, lpbID, driverBID)
	require.NoError(t, err)

	// 1. IDOR: Driver A tidak boleh melihat atau memanipulasi LPB milik Driver B.
	myA := e.get(driverA, "/api/v1/tracking/my-deliveries")
	require.Equal(t, 200, myA.status())
	assert.Empty(t, myA.list(), "Driver A tidak boleh melihat tugas milik Driver B")

	myB := e.get(driverB, "/api/v1/tracking/my-deliveries")
	require.Equal(t, 200, myB.status())
	assert.Len(t, myB.list(), 1, "Driver B melihat tugasnya sendiri")

	assert.Equal(t, 403, e.post(driverA, fmt.Sprintf("/api/v1/tracking/lpb/%d/checkpoint", lpbID), map[string]any{
		"status": "IN_TRANSIT", "latitude": -6.9932, "longitude": 110.4203,
	}).status(), "IDOR checkpoint Driver A pada LPB Driver B wajib 403")

	assert.Equal(t, 403, e.post(driverA, fmt.Sprintf("/api/v1/tracking/lpb/%d/fail", lpbID), map[string]any{
		"reason": "Coba IDOR", "latitude": -6.9932, "longitude": 110.4203,
	}).status(), "IDOR fail Driver A pada LPB Driver B wajib 403")

	assert.Equal(t, 403, e.get(driverA, fmt.Sprintf("/api/v1/lpb/%d/pdf", lpbID)).status(),
		"IDOR cetak PDF Driver A pada LPB Driver B wajib 403")

	// 2. Upload berbahaya: HTML/XSS disamarkan sebagai JPEG, script teks sebagai PNG, & file > 5 MB wajib ditolak 422.
	uploadMultipart := func(token, path, field, filename string, content []byte, fields map[string]string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		for k, v := range fields {
			require.NoError(t, mw.WriteField(k, v))
		}
		part, err := mw.CreateFormFile(field, filename)
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
		require.NoError(t, mw.Close())

		req := httptest.NewRequest(http.MethodPost, path, &body)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		e.router.ServeHTTP(rec, req)
		return rec
	}

	deliverPath := fmt.Sprintf("/api/v1/tracking/lpb/%d/deliver", lpbID)
	deliverFields := map[string]string{
		"receiver_name": "Penerima Sah",
		"latitude":      "-6.9932",
		"longitude":     "110.4203",
	}

	recHTML := uploadMultipart(driverB, deliverPath, "pod_photo", "pod.jpg",
		[]byte("<!DOCTYPE html><html><body>not an image</body></html>"), deliverFields)
	assert.Equal(t, 422, recHTML.Code, "file HTML disamarkan sebagai .jpg wajib ditolak 422")

	recScript := uploadMultipart(driverB, deliverPath, "pod_photo", "fake.png",
		[]byte("#!/bin/sh\necho not-a-png\n"), deliverFields)
	assert.Equal(t, 422, recScript.Code, "file script disamarkan sebagai .png wajib ditolak 422")

	oversized := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, storage.MaxUploadBytes+10)...)
	recBig := uploadMultipart(driverB, deliverPath, "pod_photo", "huge.jpg", oversized, deliverFields)
	assert.Equal(t, 422, recBig.Code, "file > 5 MB wajib ditolak 422")

	// 3. Upload JPEG valid dengan nama path traversal → sukses aman dengan key UUID server.
	validJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	recOK := uploadMultipart(driverB, deliverPath, "pod_photo", "../../evil.jpg", validJPEG, deliverFields)
	require.Equal(t, 200, recOK.Code)
	assert.NotContains(t, recOK.Body.String(), "..")
	assert.NotContains(t, recOK.Body.String(), "evil.jpg")

	// 4. Signed URL lokal yang dimanipulasi wajib ditolak 403 & header keamanan terpasang.
	badFile := e.call(http.MethodGet, "/api/v1/files?key=../../etc/passwd&exp=9999999999&sig=deadbeef", nil)
	assert.Equal(t, 403, badFile.status())
	assert.Equal(t, "nosniff", badFile.rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", badFile.rec.Header().Get("X-Frame-Options"))
	assert.True(t, strings.Contains(badFile.rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'"))
}
