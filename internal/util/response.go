// Package util berisi helper lintas layer yang tidak terikat satu resource:
// format response JSON, JWT, hashing password/token, baca path param.
package util

import (
	"encoding/json"
	"net/http"
)

// SuccessResponse adalah bentuk response sukses standar: data utama di "data",
// "meta" opsional untuk info pagination pada endpoint yang me-list banyak resource.
type SuccessResponse struct {
	Data interface{} `json:"data"`
	Meta *Meta       `json:"meta,omitempty"`
}

// Meta menampung info pagination. Field lain (filter/sort) sengaja tidak
// ditambahkan sebelum ada endpoint yang benar-benar mengisinya.
type Meta struct {
	Page      int   `json:"page"`
	Limit     int   `json:"limit"`
	Total     int64 `json:"total"`
	TotalPage int   `json:"total_page"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

// WriteJSON adalah primitif paling dasar: tulis status code + body JSON apa
// adanya. Helper Write* lain di bawah semua lewat sini — jarang dipanggil
// langsung dari handler.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(data)
}

// WriteSuccess membungkus data ke {"data": ...} dengan status code custom.
func WriteSuccess(w http.ResponseWriter, status int, data interface{}) {
	WriteJSON(w, status, SuccessResponse{Data: data})
}

// WriteOK adalah WriteSuccess dengan status 200 — dipakai paling sering.
func WriteOK(w http.ResponseWriter, data interface{}) {
	WriteSuccess(w, http.StatusOK, data)
}

// WriteCreated dipakai setelah berhasil membuat resource baru (status 201).
func WriteCreated(w http.ResponseWriter, data interface{}) {
	WriteSuccess(w, http.StatusCreated, data)
}

// WriteNoContent dipakai untuk aksi berhasil tanpa body (mis. logout), status 204.
func WriteNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// WriteSuccessPaginated membungkus list data + info pagination, status 200.
func WriteSuccessPaginated(w http.ResponseWriter, data interface{}, meta Meta) {
	WriteJSON(w, http.StatusOK, SuccessResponse{Data: data, Meta: &meta})
}

// WriteError menulis error sesuai format wajib di CLAUDE.md: {"error":{"code","message"}}.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}

// Helper di bawah ini membungkus WriteError untuk kasus umum, supaya handler
// tidak perlu hardcode status+code berulang-ulang di tiap resource.

func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, "validation_error", message)
}

func WriteUnauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, "unauthorized", message)
}

func WriteForbidden(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusForbidden, "forbidden", message)
}

func WriteNotFound(w http.ResponseWriter, code, message string) {
	WriteError(w, http.StatusNotFound, code, message)
}

func WriteConflict(w http.ResponseWriter, code, message string) {
	WriteError(w, http.StatusConflict, code, message)
}

func WriteMethodNotAllowed(w http.ResponseWriter) {
	WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method tidak didukung untuk endpoint ini")
}

func WriteInternalError(w http.ResponseWriter) {
	WriteError(w, http.StatusInternalServerError, "internal_error", "terjadi kesalahan internal")
}
