package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	sampleJPEG = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	samplePNG  = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	sampleWebP = []byte{'R', 'I', 'F', 'F', 0x24, 0x00, 0x00, 0x00, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}
	samplePDF  = []byte("%PDF-1.4\n%test\n%%EOF\n")
)

func TestLocalStorage_UploadOpenDelete(t *testing.T) {
	ctx := context.Background()
	st, err := NewLocal(LocalConfig{
		BaseDir:       t.TempDir(),
		BaseURL:       "http://localhost:8080",
		SigningSecret: "super-secret-test-key-32-chars!!",
	})
	require.NoError(t, err)

	cases := []struct {
		name     string
		data     []byte
		wantMIME string
		wantExt  string
	}{
		{"jpeg", sampleJPEG, "image/jpeg", ".jpg"},
		{"png", samplePNG, "image/png", ".png"},
		{"webp", sampleWebP, "image/webp", ".webp"},
		{"pdf", samplePDF, "application/pdf", ".pdf"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := st.Upload(ctx, "pod/2026", bytes.NewReader(tc.data))
			require.NoError(t, err)
			assert.Equal(t, tc.wantMIME, res.ContentType)
			assert.Equal(t, int64(len(tc.data)), res.Size)
			assert.True(t, strings.HasPrefix(res.Key, "pod/2026/"))
			assert.True(t, strings.HasSuffix(res.Key, tc.wantExt))

			rc, ct, err := st.Open(ctx, res.Key)
			require.NoError(t, err)
			got, err := io.ReadAll(rc)
			require.NoError(t, rc.Close())
			require.NoError(t, err)
			assert.Equal(t, tc.data, got)
			assert.Equal(t, tc.wantMIME, ct)

			require.NoError(t, st.Delete(ctx, res.Key))
			// Delete idempoten saat file sudah hilang.
			require.NoError(t, st.Delete(ctx, res.Key))
			_, _, err = st.Open(ctx, res.Key)
			assert.ErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestLocalStorage_SecurityValidations(t *testing.T) {
	ctx := context.Background()
	st, err := NewLocal(LocalConfig{
		BaseDir:       t.TempDir(),
		SigningSecret: "super-secret-test-key-32-chars!!",
	})
	require.NoError(t, err)

	t.Run("tolak file kosong", func(t *testing.T) {
		_, err := st.Upload(ctx, "pod", bytes.NewReader(nil))
		assert.ErrorIs(t, err, ErrEmptyFile)
	})

	t.Run("tolak file > 5 MB", func(t *testing.T) {
		big := append([]byte{}, sampleJPEG...)
		big = append(big, make([]byte, MaxUploadBytes)...)
		_, err := st.Upload(ctx, "pod", bytes.NewReader(big))
		assert.ErrorIs(t, err, ErrFileTooLarge)
	})

	t.Run("tolak MIME berbahaya (HTML/script disamarkan)", func(t *testing.T) {
		_, err := st.Upload(ctx, "pod", strings.NewReader("<!DOCTYPE html><script>alert(1)</script>"))
		assert.ErrorIs(t, err, ErrInvalidMimeType)
	})

	t.Run("tolak path traversal pada folder dan key", func(t *testing.T) {
		_, err := st.Upload(ctx, "../etc", bytes.NewReader(sampleJPEG))
		assert.ErrorIs(t, err, ErrInvalidKey)

		_, err = st.SignedURL(ctx, "../../secret.txt", time.Minute)
		assert.ErrorIs(t, err, ErrInvalidKey)

		err = st.Delete(ctx, "../outside.jpg")
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
}

func TestLocalStorage_SignedURL(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC)
	st, err := NewLocal(LocalConfig{
		BaseDir:       t.TempDir(),
		BaseURL:       "http://localhost:8080",
		SigningSecret: "super-secret-test-key-32-chars!!",
	})
	require.NoError(t, err)
	st.now = func() time.Time { return now }

	res, err := st.Upload(ctx, "pod", bytes.NewReader(sampleJPEG))
	require.NoError(t, err)

	signed, err := st.SignedURL(ctx, res.Key, 10*time.Minute)
	require.NoError(t, err)

	parsed, err := url.Parse(signed)
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/files", parsed.Path)

	q := parsed.Query()
	exp, err := strconv.ParseInt(q.Get("exp"), 10, 64)
	require.NoError(t, err)

	key, err := st.VerifySignedURL(q.Get("key"), exp, q.Get("sig"))
	require.NoError(t, err)
	assert.Equal(t, res.Key, key)

	// Signature dimodifikasi → ditolak.
	_, err = st.VerifySignedURL(q.Get("key"), exp, "deadbeef")
	assert.ErrorIs(t, err, ErrInvalidSignature)

	// Key dimodifikasi → ditolak.
	_, err = st.VerifySignedURL("pod/other.jpg", exp, q.Get("sig"))
	assert.ErrorIs(t, err, ErrInvalidSignature)

	// Lewat masa berlaku → ditolak.
	st.now = func() time.Time { return now.Add(11 * time.Minute) }
	_, err = st.VerifySignedURL(q.Get("key"), exp, q.Get("sig"))
	assert.ErrorIs(t, err, ErrInvalidSignature)
}

func TestS3Storage_UploadDeleteAndPresign(t *testing.T) {
	ctx := context.Background()
	var gotMethod, gotPath, gotCT string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("ETag", `"mock-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st, err := NewS3(S3Config{
		Endpoint:       srv.URL,
		Region:         "ap-southeast-1",
		Bucket:         "erp-files",
		AccessKey:      "test-access",
		SecretKey:      "test-secret",
		ForcePathStyle: true,
	})
	require.NoError(t, err)

	res, err := st.Upload(ctx, "pod", bytes.NewReader(samplePNG))
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "/erp-files/"+res.Key, gotPath)
	assert.Equal(t, "image/png", gotCT)
	assert.Equal(t, samplePNG, gotBody)

	signedGet, err := st.SignedURL(ctx, res.Key, 5*time.Minute)
	require.NoError(t, err)
	assert.Contains(t, signedGet, "/erp-files/"+res.Key)
	assert.Contains(t, signedGet, "X-Amz-Signature=")
	assert.Contains(t, signedGet, "X-Amz-Expires=300")

	signedPut, err := st.SignedUploadURL(ctx, "tickets/sample.jpg", "image/jpeg", 5*time.Minute)
	require.NoError(t, err)
	assert.Contains(t, signedPut, "/erp-files/tickets/sample.jpg")
	assert.Contains(t, signedPut, "X-Amz-Signature=")

	_, err = st.SignedUploadURL(ctx, "tickets/bad.html", "text/html", 5*time.Minute)
	assert.ErrorIs(t, err, ErrInvalidMimeType)

	require.NoError(t, st.Delete(ctx, res.Key))
	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "/erp-files/"+res.Key, gotPath)
}

func TestS3Storage_ConfigValidation(t *testing.T) {
	_, err := NewS3(S3Config{AccessKey: "a", SecretKey: "b"})
	assert.Error(t, err)
	_, err = NewS3(S3Config{Bucket: "erp-files"})
	assert.Error(t, err)
}
