// Package pdf merender HTML menjadi PDF melalui Gotenberg 8 (Chromium).
package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	maxErrorBody   = 512
)

// ErrUnavailable menandakan layanan Gotenberg tidak dapat dihubungi atau menolak permintaan.
var ErrUnavailable = errors.New("layanan PDF tidak tersedia")

// Options mengatur ukuran kertas dan margin (dalam inci, sesuai API Chromium Gotenberg 8).
type Options struct {
	PaperWidth      string // default A4: "8.27"
	PaperHeight     string // default A4: "11.69"
	MarginTop       string // default "0.5"
	MarginBottom    string // default "0.5"
	MarginLeft      string // default "0.5"
	MarginRight     string // default "0.5"
	Landscape       bool
	PrintBackground bool
}

// A4Options mengembalikan pengaturan standar kertas A4 portrait dengan latar cetak aktif.
func A4Options() Options {
	return Options{
		PaperWidth:      "8.27",
		PaperHeight:     "11.69",
		MarginTop:       "0.45",
		MarginBottom:    "0.45",
		MarginLeft:      "0.45",
		MarginRight:     "0.45",
		PrintBackground: true,
	}
}

// Renderer mengabstraksikan konversi HTML ke bytes PDF agar mudah di-mock di test.
type Renderer interface {
	RenderHTML(ctx context.Context, html string, opts Options) ([]byte, error)
}

// GotenbergClient memanggil endpoint POST /forms/chromium/convert/html milik Gotenberg 8.
type GotenbergClient struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

// NewGotenbergClient membuat klien Gotenberg dengan timeout 30 detik (plan Bagian 7.6).
func NewGotenbergClient(baseURL, username, password string) *GotenbergClient {
	return &GotenbergClient{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		username: username,
		password: password,
		client:   &http.Client{Timeout: defaultTimeout},
	}
}

var _ Renderer = (*GotenbergClient)(nil)

// RenderHTML mengirim index.html sebagai multipart form ke Gotenberg dan mengembalikan bytes PDF.
func (g *GotenbergClient) RenderHTML(ctx context.Context, html string, opts Options) ([]byte, error) {
	if g.baseURL == "" {
		return nil, fmt.Errorf("%w: GOTENBERG_URL kosong", ErrUnavailable)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	part, err := mw.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, fmt.Errorf("pdf: buat form index.html: %w", err)
	}
	if _, err := io.WriteString(part, html); err != nil {
		return nil, fmt.Errorf("pdf: tulis html: %w", err)
	}

	writeField := func(k, v string) error {
		if v == "" {
			return nil
		}
		return mw.WriteField(k, v)
	}
	for _, kv := range [][2]string{
		{"paperWidth", opts.PaperWidth},
		{"paperHeight", opts.PaperHeight},
		{"marginTop", opts.MarginTop},
		{"marginBottom", opts.MarginBottom},
		{"marginLeft", opts.MarginLeft},
		{"marginRight", opts.MarginRight},
	} {
		if err := writeField(kv[0], kv[1]); err != nil {
			return nil, fmt.Errorf("pdf: tulis field %s: %w", kv[0], err)
		}
	}
	if opts.Landscape {
		if err := mw.WriteField("landscape", "true"); err != nil {
			return nil, err
		}
	}
	if opts.PrintBackground {
		if err := mw.WriteField("printBackground", "true"); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("pdf: tutup multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/forms/chromium/convert/html", &body)
	if err != nil {
		return nil, fmt.Errorf("pdf: buat request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if g.username != "" || g.password != "" {
		req.SetBasicAuth(g.username, g.password)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return nil, fmt.Errorf("%w: gotenberg HTTP %d: %s", ErrUnavailable, resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: baca respons pdf: %v", ErrUnavailable, err)
	}
	return out, nil
}
