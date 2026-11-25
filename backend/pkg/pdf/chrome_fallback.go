package pdf

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// LocalChromeRenderer merender HTML menjadi PDF secara lokal memakai Chrome/Edge headless
// sebagai fallback development saat container Gotenberg belum dijalankan.
type LocalChromeRenderer struct {
	binPath string
}

// NewLocalChromeRenderer mencari binary Chrome/Edge/Chromium lokal di mesin developer.
func NewLocalChromeRenderer() *LocalChromeRenderer {
	return &LocalChromeRenderer{binPath: detectLocalBrowser()}
}

var _ Renderer = (*LocalChromeRenderer)(nil)

func (r *LocalChromeRenderer) Available() bool { return r.binPath != "" }

func (r *LocalChromeRenderer) RenderHTML(ctx context.Context, html string, _ Options) ([]byte, error) {
	if r.binPath == "" {
		return nil, fmt.Errorf("%w: browser Chromium/Chrome/Edge lokal tidak ditemukan", ErrUnavailable)
	}
	tmpDir, err := os.MkdirTemp("", "erp-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("pdf local: mkdir temp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inFile := filepath.Join(tmpDir, "index.html")
	outFile := filepath.Join(tmpDir, "out.pdf")
	if err := os.WriteFile(inFile, []byte(html), 0o600); err != nil {
		return nil, fmt.Errorf("pdf local: write html: %w", err)
	}

	fileURL := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(inFile)}).String()
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--no-pdf-header-footer",
		"--print-to-pdf=" + outFile,
		fileURL,
	}
	cmd := exec.CommandContext(ctx, r.binPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%w: chrome headless gagal: %v (%s)", ErrUnavailable, err, string(out))
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		return nil, fmt.Errorf("%w: baca hasil pdf lokal: %v", ErrUnavailable, err)
	}
	return data, nil
}

// FallbackRenderer mencoba primary (Gotenberg) terlebih dahulu; jika tidak tersedia,
// jatuh ke fallback (Chrome/Edge lokal saat APP_ENV=local).
type FallbackRenderer struct {
	primary  Renderer
	fallback Renderer
}

func NewFallbackRenderer(primary, fallback Renderer) *FallbackRenderer {
	return &FallbackRenderer{primary: primary, fallback: fallback}
}

var _ Renderer = (*FallbackRenderer)(nil)

func (f *FallbackRenderer) RenderHTML(ctx context.Context, html string, opts Options) ([]byte, error) {
	data, err := f.primary.RenderHTML(ctx, html, opts)
	if err == nil {
		return data, nil
	}
	if errors.Is(err, ErrUnavailable) && f.fallback != nil {
		if fbData, fbErr := f.fallback.RenderHTML(ctx, html, opts); fbErr == nil {
			return fbData, nil
		}
	}
	return nil, err
}

func detectLocalBrowser() string {
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		}
	} else {
		candidates = []string{
			"google-chrome",
			"chromium",
			"chromium-browser",
			"microsoft-edge",
		}
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
