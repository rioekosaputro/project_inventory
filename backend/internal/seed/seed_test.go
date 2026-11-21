package seed

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_ValidatesInputBeforeTouchingDB(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{"email kosong", Options{AdminPassword: "password-kuat"}, "SEED_ADMIN_EMAIL"},
		{"email tanpa @", Options{AdminEmail: "admin", AdminPassword: "password-kuat"}, "SEED_ADMIN_EMAIL"},
		{"password kosong", Options{AdminEmail: "a@x.id"}, "SEED_ADMIN_PASSWORD"},
		{"password pendek", Options{AdminEmail: "a@x.id", AdminPassword: "pendek"}, "SEED_ADMIN_PASSWORD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Run(context.Background(), nil, tt.opts) // pool nil: harus gagal sebelum dipakai
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestDefaultSettings_ValidJSONAndDerivedVerifyURL(t *testing.T) {
	s, err := defaultSettings("admin@x.id", "http://localhost:5173/")
	require.NoError(t, err)

	byKey := map[string]string{}
	for _, it := range s {
		assert.True(t, json.Valid([]byte(it.value)), it.key)
		byKey[it.key] = it.value
	}
	assert.Equal(t, `"http://localhost:5173/verify/lpb"`, byKey["pdf.verify_base_url"])
	assert.Equal(t, "false", byKey["lpb.allow_self_approve"])
	assert.Contains(t, byKey["company.profile"], "admin@x.id")
}
