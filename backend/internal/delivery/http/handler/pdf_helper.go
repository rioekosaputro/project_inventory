package handler

import (
	"bytes"
	"context"
	"fmt"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/pkg/pdf"
)

// pdfContext mengaktifkan fallback HTML siap-cetak bila klien mengirim ?fallback=html.
// Tanpa parameter tersebut, endpoint tetap mengembalikan 503 bila Gotenberg tidak tersedia.
func pdfContext(c *gin.Context) context.Context {
	ctx := c.Request.Context()
	if c.Query("fallback") == "html" {
		return pdf.WithHTMLFallback(ctx)
	}
	return ctx
}

// writePDFOrHTML mengirim bytes PDF (application/pdf) atau fallback HTML siap-cetak (text/html).
func writePDFOrHTML(c *gin.Context, filename string, data []byte) {
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		disp := "inline"
		if c.Query("download") == "1" || c.Query("download") == "true" {
			disp = "attachment"
		}
		c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disp, filename))
		c.Data(nethttp.StatusOK, "application/pdf", data)
		return
	}
	c.Header("X-PDF-Fallback", "html")
	c.Data(nethttp.StatusOK, "text/html; charset=utf-8", data)
}
