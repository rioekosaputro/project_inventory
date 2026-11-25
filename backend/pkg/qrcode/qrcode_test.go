package qrcode

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataURI(t *testing.T) {
	uri, err := DataURI("https://erp.contoh.id/verify/lpb/abc123", 180)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(uri, "data:image/png;base64,"))

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:image/png;base64,"))
	require.NoError(t, err)
	// Magic bytes PNG: 89 50 4E 47
	require.Greater(t, len(raw), 8)
	assert.Equal(t, []byte{0x89, 'P', 'N', 'G'}, raw[:4])
}

func TestDataURI_EmptyContent(t *testing.T) {
	_, err := DataURI("", 180)
	assert.Error(t, err)
}
