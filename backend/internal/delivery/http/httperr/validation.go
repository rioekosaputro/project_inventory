package httperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	"rental-inventory-api/pkg/response"
)

var setupOnce sync.Once

// Setup membuat validator melaporkan nama field sesuai tag json (bukan nama field Go).
// Aman dipanggil berulang; dipanggil otomatis oleh NewRouter.
func Setup() {
	setupOnce.Do(func() {
		v, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			for _, tag := range []string{"json", "form", "uri"} {
				name := strings.SplitN(f.Tag.Get(tag), ",", 2)[0]
				if name == "-" {
					return ""
				}
				if name != "" {
					return name
				}
			}
			return ""
		})
	})
}

// translateBindError menerjemahkan error binding/validasi/JSON. ok=false bila bukan error binding.
func translateBindError(err error) (status int, code, message string, fields []response.FieldError, ok bool) {
	var maxErr *nethttp.MaxBytesError
	if errors.As(err, &maxErr) {
		return nethttp.StatusRequestEntityTooLarge, CodePayloadTooLarge, "Ukuran data yang dikirim terlalu besar", nil, true
	}

	var vErrs validator.ValidationErrors
	if errors.As(err, &vErrs) {
		for _, fe := range vErrs {
			fields = append(fields, response.FieldError{Field: fieldPath(fe), Message: fieldMessage(fe)})
		}
		return nethttp.StatusBadRequest, CodeValidation, "Data yang dikirim tidak valid", fields, true
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		fields = []response.FieldError{{Field: typeErr.Field, Message: "harus berupa " + kindLabel(typeErr.Type)}}
		return nethttp.StatusBadRequest, CodeValidation, "Data yang dikirim tidak valid", fields, true
	}

	var syntaxErr *json.SyntaxError
	switch {
	case errors.Is(err, io.EOF):
		return nethttp.StatusBadRequest, CodeBadRequest, "Body request kosong", nil, true
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return nethttp.StatusBadRequest, CodeBadRequest, "Format JSON tidak valid", nil, true
	}
	return 0, "", "", nil, false
}

// fieldPath: "CreateReq.items[0].qty" → "items[0].qty".
func fieldPath(fe validator.FieldError) string {
	ns := fe.Namespace()
	if i := strings.Index(ns, "."); i >= 0 {
		return ns[i+1:]
	}
	return ns
}

func kindLabel(t reflect.Type) string {
	if t == nil {
		return "nilai yang valid"
	}
	switch t.Kind() {
	case reflect.String:
		return "teks"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "angka bulat"
	case reflect.Float32, reflect.Float64:
		return "angka"
	case reflect.Bool:
		return "true atau false"
	case reflect.Slice, reflect.Array:
		return "daftar"
	case reflect.Struct, reflect.Map:
		return "objek"
	default:
		return "nilai yang valid"
	}
}

// fieldMessage menerjemahkan tag validator ke Bahasa Indonesia.
func fieldMessage(fe validator.FieldError) string {
	p := fe.Param()
	switch fe.Tag() {
	case "required", "required_if", "required_unless", "required_with", "required_without":
		return "wajib diisi"
	case "email":
		return "format email tidak valid"
	case "url", "uri", "http_url":
		return "format URL tidak valid"
	case "uuid", "uuid4":
		return "format UUID tidak valid"
	case "numeric", "number":
		return "harus berupa angka"
	case "alphanum":
		return "hanya boleh huruf dan angka"
	case "alpha":
		return "hanya boleh huruf"
	case "datetime":
		return "format tanggal/waktu tidak valid (diharapkan " + p + ")"
	case "oneof":
		return "harus salah satu dari: " + strings.Join(strings.Fields(p), ", ")
	case "eqfield":
		return "harus sama dengan " + p
	case "nefield":
		return "tidak boleh sama dengan " + p
	case "min", "gte":
		return "minimal " + sizeLabel(fe, p)
	case "max", "lte":
		return "maksimal " + sizeLabel(fe, p)
	case "len":
		return "harus tepat " + sizeLabel(fe, p)
	case "gt":
		return "harus lebih dari " + sizeLabel(fe, p)
	case "lt":
		return "harus kurang dari " + sizeLabel(fe, p)
	default:
		return "tidak valid"
	}
}

// sizeLabel: panjang untuk string/koleksi, nilai untuk angka.
func sizeLabel(fe validator.FieldError, param string) string {
	switch fe.Kind() {
	case reflect.String:
		return param + " karakter"
	case reflect.Slice, reflect.Array, reflect.Map:
		return fmt.Sprintf("%s item", param)
	default:
		return param
	}
}
