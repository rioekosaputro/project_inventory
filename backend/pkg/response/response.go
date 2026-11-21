// Package response menulis envelope JSON standar (plan Bagian 7.4):
//
//	sukses: {"success":true,"message":"...","data":...,"meta":...}
//	gagal : {"success":false,"message":"...","code":"...","errors":[...],"request_id":"..."}
package response

import (
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"

	"rental-inventory-api/pkg/reqctx"
)

const defaultOKMessage = "Berhasil"

// FieldError adalah detail kesalahan per field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type successBody struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	Meta    any    `json:"meta,omitempty"`
}

type errorBody struct {
	Success   bool         `json:"success"`
	Message   string       `json:"message"`
	Code      string       `json:"code"`
	Errors    []FieldError `json:"errors,omitempty"`
	RequestID string       `json:"request_id,omitempty"`
}

// OK menulis 200 dengan data.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, successBody{Success: true, Message: defaultOKMessage, Data: data})
}

// OKMessage menulis 200 dengan pesan khusus.
func OKMessage(c *gin.Context, message string, data any) {
	c.JSON(http.StatusOK, successBody{Success: true, Message: message, Data: data})
}

// Created menulis 201 dengan data sumber daya baru.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, successBody{Success: true, Message: "Data berhasil dibuat", Data: data})
}

// Paginated menulis 200 untuk daftar beserta meta paginasi.
// Slice nil dikirim sebagai [] (bukan null) agar FE tidak perlu menangani dua bentuk.
func Paginated(c *gin.Context, data any, meta any) {
	c.JSON(http.StatusOK, successBody{Success: true, Message: defaultOKMessage, Data: emptyIfNilSlice(data), Meta: meta})
}

// Error menulis respons gagal dan menghentikan rantai handler.
func Error(c *gin.Context, status int, code, message string, errs []FieldError) {
	c.AbortWithStatusJSON(status, errorBody{
		Success:   false,
		Message:   message,
		Code:      code,
		Errors:    errs,
		RequestID: reqctx.RequestID(c.Request.Context()),
	})
}

func emptyIfNilSlice(data any) any {
	if data == nil {
		return []any{}
	}
	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Slice && v.IsNil() {
		return reflect.MakeSlice(v.Type(), 0, 0).Interface()
	}
	return data
}
