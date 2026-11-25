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

var (
	lpbSuratJalanTmpl = template.Must(template.ParseFS(fs, "lpb_surat_jalan.html"))
	labelAssetTmpl    = template.Must(template.ParseFS(fs, "label_asset.html"))
)

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

// LabelItem adalah satu stiker label (asset tag unit atau kode rak).
type LabelItem struct {
	Kind      string       // mis. "ASET" atau "RAK"
	Code      string       // kode utama yang di-encode ke QR (asset tag / kode rak)
	Subtitle  string       // nama produk / kode gudang
	Meta      string       // SN / zona
	Badge     string       // opsional (mis. "NON-SELLABLE")
	QRDataURI template.URL // data:image/png;base64,...
}

// LabelsData adalah view-model untuk template label_asset.html.
type LabelsData struct {
	Title  string
	Labels []LabelItem
}

// RenderLabels merender lembar stiker label (asset tag atau kode rak) siap cetak A4.
func RenderLabels(data LabelsData) (string, error) {
	var buf bytes.Buffer
	if err := labelAssetTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("templates: render label_asset: %w", err)
	}
	return buf.String(), nil
}
