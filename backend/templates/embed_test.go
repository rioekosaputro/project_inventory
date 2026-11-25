package templates

import (
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderLPBSuratJalan(t *testing.T) {
	html, err := RenderLPBSuratJalan(LPBSuratJalanData{
		Title:          "SURAT JALAN / LPB KELUAR",
		LPBNumber:      "LPBO/WH-A/2026/11/0001",
		TypeLabel:      "Keluar",
		SubTypeLabel:   "Transfer Keluar",
		DocDate:        "25 Nov 2026",
		WarehouseCode:  "WH-A",
		WarehouseName:  "Gudang Semarang",
		Notes:          "<script>alert(1)</script>",
		ItemCount:      1,
		TotalQty:       2,
		CreatedByName:  "Budi",
		ApprovedByName: "Siti",
		QRDataURI:      template.URL("data:image/png;base64,iVBORw0KGgo="),
		Items: []LPBSuratJalanItem{
			{No: 1, SKU: "PRN-1", ProductName: "Epson L3210", Unit: "unit", RackCode: "A-01", Qty: 2, Serials: []string{"AST-2611-00001 (SN-1)"}},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, html, "LPBO/WH-A/2026/11/0001")
	assert.Contains(t, html, "AST-2611-00001 (SN-1)")
	assert.Contains(t, html, "data:image/png;base64,iVBORw0KGgo=")
	// Pastikan catatan user di-escape agar tidak terjadi HTML/XSS injection ke Chromium Gotenberg.
	assert.NotContains(t, html, "<script>alert(1)</script>")
	assert.Contains(t, html, "&lt;script&gt;alert(1)&lt;/script&gt;")
}

func TestRenderLabels(t *testing.T) {
	html, err := RenderLabels(LabelsData{
		Title: "Label Aset",
		Labels: []LabelItem{
			{
				Kind:      "ASET",
				Code:      "AST-2611-00001",
				Subtitle:  "Epson L3210",
				Meta:      "SN: SN-001",
				QRDataURI: template.URL("data:image/png;base64,iVBORw0KGgo="),
			},
			{
				Kind:      "RAK",
				Code:      "A-02-01",
				Subtitle:  "WH-A",
				Meta:      "Zona: KARANTINA",
				Badge:     "NON-SELLABLE",
				QRDataURI: template.URL("data:image/png;base64,iVBORw0KGgo="),
			},
		},
	})
	require.NoError(t, err)
	assert.Contains(t, html, "AST-2611-00001")
	assert.Contains(t, html, "SN: SN-001")
	assert.Contains(t, html, "A-02-01")
	assert.Contains(t, html, "NON-SELLABLE")
}
