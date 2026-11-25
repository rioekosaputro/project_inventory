//go:build integration

package http_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"rental-inventory-api/internal/testutil/pgtest"
)

func (e *env) uploadInitialStock(token string, whID int64, filename string, content []byte) *reply {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(e.t, mw.WriteField("warehouse_id", fmt.Sprintf("%d", whID)))
	fw, err := mw.CreateFormFile("file", filename)
	require.NoError(e.t, err)
	_, err = fw.Write(content)
	require.NoError(e.t, err)
	require.NoError(e.t, mw.Close())

	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/lpb/import-initial-stock", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = defaultIP + ":1234"
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return &reply{rec: rec, body: body}
}

func TestLPB_ImportInitialStock_TemplateCSVXLSXAndValidation(t *testing.T) {
	e := newEnv(t)
	f := newLPBFixture(t, e)

	// 1. Download template Excel (.xlsx).
	tmpl := e.get(f.staffA, "/api/v1/lpb/import-initial-stock/template")
	require.Equal(t, 200, tmpl.status())
	assert.Contains(t, tmpl.rec.Header().Get("Content-Type"), "spreadsheetml.sheet")
	xlT, err := excelize.OpenReader(bytes.NewReader(tmpl.rec.Body.Bytes()))
	require.NoError(t, err)
	defer func() { _ = xlT.Close() }()
	rowsT, err := xlT.GetRows("Stok Awal")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(rowsT), 2)
	assert.Equal(t, "sku", rowsT[0][0])

	// 2. Import CSV (non-SN + SN) → menghasilkan DRAFT LPB INITIAL_STOCK → submit → approve → invariant 0.
	csvData := "sku,rack_code,qty,serial_number,condition,unit_cost,notes\n" +
		"TN-1,A-01,10,,,120000.00,Stok awal toner\n" +
		"PRN-1,A-01,1,SN-IMP-1,GOOD,2000000,Printer 1\n" +
		"PRN-1,A-01,1,SN-IMP-2,MINOR_DEFECT,2000000,Printer 2\n"

	imp := e.uploadInitialStock(f.staffA, f.whA, "stok_awal.csv", []byte(csvData))
	require.Equal(t, 201, imp.status(), imp.rec.Body.String())
	lpbID := idOf(imp)
	assert.Equal(t, "DRAFT", imp.str("status"))
	assert.Equal(t, "INITIAL_STOCK", imp.str("sub_type"))
	assert.Len(t, itemsOf(imp), 2)
	// Tahap import (DRAFT) belum mengubah stok!
	assert.Equal(t, 0, e.bal(f.p1, f.a1))
	assert.Equal(t, 0, e.bal(f.p2, f.a1))

	// Submit & approve → stok & unit terdaftar.
	require.Equal(t, 200, e.post(f.staffA, lpbPath(lpbID, "/submit"), nil).status())
	require.Equal(t, 200, e.approve(f.headA, lpbID, nextKey()).status())
	assert.Equal(t, 2, e.bal(f.p1, f.a1))
	assert.Equal(t, 10, e.bal(f.p2, f.a1))
	pgtest.AssertInvariants(t, e.pool)

	// 3. Validasi per baris: SKU tak dikenal + SN sudah terdaftar → 422, tidak membuat LPB baru.
	badCSV := "sku,rack_code,qty,serial_number,condition,unit_cost,notes\n" +
		"SKU-GA-ADA,A-01,5,,,,\n" +
		"PRN-1,A-01,1,SN-IMP-1,GOOD,,\n"
	bad := e.uploadInitialStock(f.staffA, f.whA, "bad.csv", []byte(badCSV))
	assert.Equal(t, 422, bad.status(), bad.rec.Body.String())
	assert.Contains(t, fieldErrors(bad), "row[2].sku")
	assert.Contains(t, fieldErrors(bad), "row[3].serial_number")
}
