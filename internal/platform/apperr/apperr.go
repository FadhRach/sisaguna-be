// Package apperr membungkus error service dengan field tambahan untuk respons API
// (mis. suggested_tier, harga terbaru) tanpa service perlu mengenal HTTP.
package apperr

type withFields struct {
	error
	fields map[string]any
}

// WithFields membungkus err; errors.Is(wrapped, err) tetap true.
func WithFields(err error, fields map[string]any) error {
	return withFields{error: err, fields: fields}
}

func (w withFields) Unwrap() error { return w.error }

// ErrorFields dibaca httpx.Fail untuk digabung ke objek error di respons.
func (w withFields) ErrorFields() map[string]any { return w.fields }
