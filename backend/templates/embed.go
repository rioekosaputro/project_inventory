// Package templates menyematkan template HTML dokumen PDF (plan Bagian 7.6) menggunakan go:embed
// agar selalu terbawa di dalam binary (penting untuk container & serverless).
package templates

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

//go:embed *.html
var fs embed.FS

var lpbSuratJalanTmpl = template.Must(template.ParseFS(fs, "lpb_surat_jalan.html"))

// LPBSuratJalanItem merepresentasikan satu baris item pada PDF Surat Jalan / LPB.
type LPBSuratJalanItem struct {
	No          int
	SKU         string
	ProductName string
	Unit        string
	RackCode    string
	DestLabel   string // diisi untuk TRANSFER_OUT (mis. "WH-B / B-01")
	Qty         int
	Serials     []string // format "AST-... (SN-...)" atau "SN-..."
	Notes       string
}

// LPBSuratJalanData adalah data view-model untuk template lpb_surat_jalan.html.
type LPBSuratJalanData struct {
	Title             string
	LPBNumber         string
	TypeLabel         string
	SubTypeLabel      string
	DocDate           string
	WarehouseCode     string
	WarehouseName     string
	WarehouseAddress  string
	SupplierName      string
	ReferenceNumber   string
	CounterpartNumber string
	Notes             string
	ItemCount         int
	TotalQty          int
	Items             []LPBSuratJalanItem
	CreatedByName     string
	CreatedAt         string
	ApprovedByName    string
	ApprovedAt        string
	PrintedAt         string
	QRDataURI         template.URL // data:image/png;base64,...
}

// RenderLPBSuratJalan merender HTML siap cetak untuk PDF Surat Jalan / LPB.
func RenderLPBSuratJalan(data LPBSuratJalanData) (string, error) {
	var buf bytes.Buffer
	if err := lpbSuratJalanTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("templates: render lpb_surat_jalan: %w", err)
	}
	return buf.String(), nil
}
