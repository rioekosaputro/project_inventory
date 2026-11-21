package usecase

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"rental-inventory-api/internal/domain"
)

func strp(s string) *string { return &s }

func TestParseMoney(t *testing.T) {
	valid := map[string]string{
		"0": "0.00", "550000": "550000.00", "550000.5": "550000.50", "550000.50": "550000.50",
		" 12.34 ": "12.34", "9999999999999.99": "9999999999999.99",
	}
	for in, want := range valid {
		d, errs := parseMoney("f", strp(in))
		require.Empty(t, errs, in)
		require.NotNil(t, d, in)
		assert.Equal(t, want, d.StringFixed(2), in)
	}

	for _, in := range []string{"-1", "1e3", "1,5", "12.345", "abc", "1.", ".5", "+5", "99999999999999", "0x10", "1 000"} {
		d, errs := parseMoney("f", strp(in))
		assert.Nil(t, d, in)
		require.Len(t, errs, 1, in)
		assert.Equal(t, "f", errs[0].Field)
	}

	for _, empty := range []*string{nil, strp(""), strp("   ")} {
		d, errs := parseMoney("f", empty)
		assert.Nil(t, d)
		assert.Empty(t, errs, "kosong = tidak diisi")
	}
}

func TestMoneyViewIsStable(t *testing.T) {
	d := decimal.RequireFromString("1250000.5")
	assert.Equal(t, "1250000.50", decVal(&d))
	assert.Nil(t, decVal(nil))
}

func TestApplyPatch(t *testing.T) {
	cur := strp("lama")
	assert.Equal(t, cur, applyPatch(cur, Patch[string]{}), "tidak dikirim → tetap")
	assert.Nil(t, applyPatch(cur, Patch[string]{Set: true}), "null → dikosongkan")
	assert.Nil(t, applyPatch(cur, Patch[string]{Set: true, Value: strp("   ")}), "spasi → dikosongkan")
	assert.Equal(t, "baru", *applyPatch(cur, Patch[string]{Set: true, Value: strp(" baru ")}))
}

func TestDiffViews_OnlyChangedFields(t *testing.T) {
	a := map[string]any{"name": "A", "phone": nil, "n": 1, "x": "same"}
	b := map[string]any{"name": "B", "phone": "123", "n": 1, "x": "same"}
	before, after := diffViews(a, b)
	assert.Equal(t, map[string]any{"name": "A", "phone": nil}, before)
	assert.Equal(t, map[string]any{"name": "B", "phone": "123"}, after)

	before, after = diffViews(a, a)
	assert.Empty(t, before)
	assert.Empty(t, after)
}

func TestPtrVal_DistinguishesNilFromEmpty(t *testing.T) {
	var np *string
	assert.Nil(t, ptrVal(np))
	assert.Equal(t, "", ptrVal(strp("")))
	f := 1.5
	assert.Equal(t, 1.5, ptrVal(&f))
}

func TestScopeOf(t *testing.T) {
	wh := int64(7)
	assert.Nil(t, scopeOf(domain.Principal{Role: domain.RoleAdmin, WarehouseID: &wh}), "ADMIN tidak pernah dibatasi")
	assert.Nil(t, scopeOf(domain.Principal{Role: domain.RoleHeadWarehouse}), "tanpa warehouse_id = semua gudang")
	got := scopeOf(domain.Principal{Role: domain.RoleStaffWarehouse, WarehouseID: &wh})
	require.NotNil(t, got)
	assert.EqualValues(t, 7, *got)
}

func TestNormalizeZoneAndSellableDefaults(t *testing.T) {
	assert.Nil(t, normalizeZone(nil))
	assert.Nil(t, normalizeZone(strp("  ")))
	assert.Equal(t, "KARANTINA", *normalizeZone(strp(" karantina ")))
	assert.True(t, nonSellableZones["KARANTINA"])
	assert.True(t, nonSellableZones["WORKSHOP"])
	assert.False(t, nonSellableZones["PRINTER"])
}

func TestCodePatterns(t *testing.T) {
	for _, ok := range []string{"WH-SMG-01", "A1", "SUP-001", "X"} {
		assert.True(t, codePattern.MatchString(ok), ok)
	}
	for _, bad := range []string{"", "wh-1", "WH/01", "WH 01", "-WH", "WH-", "WH--1", "WH_1"} {
		assert.False(t, codePattern.MatchString(bad), bad)
	}
	for _, ok := range []string{"A-02-01", "A.1_2", "Z9"} {
		assert.True(t, rackCodePattern.MatchString(ok), ok)
	}
	assert.False(t, rackCodePattern.MatchString("-A"))
	assert.True(t, skuPattern.MatchString("PRN-L3210/BK"))
	assert.False(t, skuPattern.MatchString("PRN 1"))
}

func TestSerialRule(t *testing.T) {
	assert.Empty(t, serialRule(domain.ItemTypeAsset, true))
	assert.Empty(t, serialRule(domain.ItemTypeAsset, false))
	assert.Empty(t, serialRule(domain.ItemTypeConsumable, false))
	assert.Len(t, serialRule(domain.ItemTypeConsumable, true), 1)
}

func TestValidateWarehouse_CoordinatePair(t *testing.T) {
	lat, lng := 10.0, 20.0
	base := domain.Warehouse{Name: "G"}

	w := base
	assert.Empty(t, validateWarehouse(&w), "tanpa koordinat valid")
	w.Latitude, w.Longitude = &lat, &lng
	assert.Empty(t, validateWarehouse(&w))
	w.Longitude = nil
	assert.NotEmpty(t, validateWarehouse(&w), "salah satu saja ditolak")
	bad := 95.0
	w.Latitude, w.Longitude = &bad, &lng
	assert.NotEmpty(t, validateWarehouse(&w))
}
