package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LocalStorage menyimpan berkas di direktori disk lokal (development / VPS single-node)
// dan menerbitkan signed URL HMAC-SHA256 berumur pendek untuk endpoint `/api/v1/files`.
type LocalStorage struct {
	baseDir       string
	baseURL       string
	signingSecret []byte
	now           func() time.Time
}

// LocalConfig mengatur penyimpanan lokal.
type LocalConfig struct {
	BaseDir       string // mis. "./storage"
	BaseURL       string // mis. "http://localhost:8080" atau "" (menghasilkan path relatif /api/v1/files)
	SigningSecret string // dipakai untuk HMAC-SHA256 signed URL
}

// NewLocal membuat instance LocalStorage.
func NewLocal(cfg LocalConfig) (*LocalStorage, error) {
	if strings.TrimSpace(cfg.BaseDir) == "" {
		return nil, errors.New("storage local: BaseDir wajib diisi")
	}
	absDir, err := filepath.Abs(cfg.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("storage local: resolve path: %w", err)
	}
	if err := os.MkdirAll(absDir, 0o750); err != nil {
		return nil, fmt.Errorf("storage local: buat direktori: %w", err)
	}
	secret := strings.TrimSpace(cfg.SigningSecret)
	if secret == "" {
		secret = "local-dev-storage-signing-secret"
	}
	return &LocalStorage{
		baseDir:       absDir,
		baseURL:       strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		signingSecret: []byte(secret),
		now:           time.Now,
	}, nil
}

var _ Uploader = (*LocalStorage)(nil)

func (l *LocalStorage) Upload(_ context.Context, folder string, r io.Reader) (*UploadResult, error) {
	prep, err := prepareUpload(folder, r)
	if err != nil {
		return nil, err
	}
	fullPath, err := l.resolvePath(prep.key)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return nil, fmt.Errorf("storage local: mkdir: %w", err)
	}
	if err := os.WriteFile(fullPath, prep.data, 0o640); err != nil {
		return nil, fmt.Errorf("storage local: tulis file: %w", err)
	}
	return &UploadResult{
		Key:         prep.key,
		ContentType: prep.contentType,
		Size:        int64(len(prep.data)),
	}, nil
}

func (l *LocalStorage) Delete(_ context.Context, key string) error {
	fullPath, err := l.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("storage local: hapus file: %w", err)
	}
	return nil
}

func (l *LocalStorage) SignedURL(_ context.Context, key string, ttl time.Duration) (string, error) {
	cleanKey, err := sanitizeKey(key)
	if err != nil {
		return "", err
	}
	exp := l.now().Add(normalizeTTL(ttl)).Unix()
	sig := l.sign("GET", cleanKey, exp)

	q := url.Values{}
	q.Set("key", cleanKey)
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", sig)
	return l.baseURL + "/api/v1/files?" + q.Encode(), nil
}

func (l *LocalStorage) SignedUploadURL(_ context.Context, key string, contentType string, ttl time.Duration) (string, error) {
	cleanKey, err := sanitizeKey(key)
	if err != nil {
		return "", err
	}
	if _, ok := allowedMIMETypes[strings.ToLower(strings.TrimSpace(contentType))]; !ok {
		return "", fmt.Errorf("%w: %s", ErrInvalidMimeType, contentType)
	}
	exp := l.now().Add(normalizeTTL(ttl)).Unix()
	sig := l.sign("PUT", cleanKey, exp)

	q := url.Values{}
	q.Set("key", cleanKey)
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", sig)
	return l.baseURL + "/api/v1/files?" + q.Encode(), nil
}

// VerifySignedURL memverifikasi parameter key, exp, dan sig untuk signed URL baca (GET).
func (l *LocalStorage) VerifySignedURL(key string, exp int64, sig string) (string, error) {
	cleanKey, err := sanitizeKey(key)
	if err != nil {
		return "", ErrInvalidSignature
	}
	if exp < l.now().Unix() {
		return "", ErrInvalidSignature
	}
	expected := l.sign("GET", cleanKey, exp)
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return "", ErrInvalidSignature
	}
	return cleanKey, nil
}

// Open membuka berkas lokal secara aman beserta MIME type hasil deteksi isi.
func (l *LocalStorage) Open(_ context.Context, key string) (io.ReadCloser, string, error) {
	fullPath, err := l.resolvePath(key)
	if err != nil {
		return nil, "", err
	}
	f, err := os.Open(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("storage local: buka file: %w", err)
	}
	buf := make([]byte, 512)
	n, readErr := f.Read(buf)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		_ = f.Close()
		return nil, "", fmt.Errorf("storage local: baca header file: %w", readErr)
	}
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		_ = f.Close()
		return nil, "", fmt.Errorf("storage local: seek awal file: %w", seekErr)
	}
	return f, detectMIME(buf[:n]), nil
}

func (l *LocalStorage) sign(method, key string, exp int64) string {
	mac := hmac.New(sha256.New, l.signingSecret)
	_, _ = fmt.Fprintf(mac, "%s\n%s\n%d", method, key, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

// resolvePath memastikan key tidak bisa keluar dari baseDir (anti path traversal).
func (l *LocalStorage) resolvePath(key string) (string, error) {
	cleanKey, err := sanitizeKey(key)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(l.baseDir, filepath.FromSlash(cleanKey))
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}
	rel, err := filepath.Rel(l.baseDir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", ErrInvalidKey
	}
	return abs, nil
}
