package pdf

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGotenbergClient_RenderHTML_Success(t *testing.T) {
	var gotPath, gotUser, gotPass, gotHTML, gotWidth, gotPrintBG string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser, gotPass, _ = r.BasicAuth()
		require.NoError(t, r.ParseMultipartForm(1<<20))
		f, fh, err := r.FormFile("files")
		require.NoError(t, err)
		defer f.Close()
		assert.Equal(t, "index.html", fh.Filename)
		b, _ := io.ReadAll(f)
		gotHTML = string(b)
		gotWidth = r.FormValue("paperWidth")
		gotPrintBG = r.FormValue("printBackground")
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4\nfake"))
	}))
	defer srv.Close()

	c := NewGotenbergClient(srv.URL, "user1", "secret1")
	out, err := c.RenderHTML(context.Background(), "<html><body>Halo</body></html>", A4Options())
	require.NoError(t, err)
	assert.Equal(t, []byte("%PDF-1.4\nfake"), out)
	assert.Equal(t, "/forms/chromium/convert/html", gotPath)
	assert.Equal(t, "user1", gotUser)
	assert.Equal(t, "secret1", gotPass)
	assert.Equal(t, "<html><body>Halo</body></html>", gotHTML)
	assert.Equal(t, "8.27", gotWidth)
	assert.Equal(t, "true", gotPrintBG)
}

func TestGotenbergClient_RenderHTML_Non200ReturnsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "chromium crash", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewGotenbergClient(srv.URL, "", "")
	_, err := c.RenderHTML(context.Background(), "<html></html>", A4Options())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnavailable)
}
