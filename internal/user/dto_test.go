package user

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"

	"github.com/FadhRach/sisaguna-be/internal/platform/httpx"
)

func TestNullable(t *testing.T) {
	var absent, null, val UpdateProfileRequest
	_ = json.Unmarshal([]byte(`{}`), &absent)
	_ = json.Unmarshal([]byte(`{"photo_media_id":null}`), &null)
	_ = json.Unmarshal([]byte(`{"photo_media_id":"`+uuid.NewString()+`"}`), &val)
	if absent.PhotoMediaID.Set || !null.PhotoMediaID.Set || null.PhotoMediaID.Valid || !val.PhotoMediaID.Valid {
		t.Errorf("absent=%+v null=%+v val=%+v", absent.PhotoMediaID, null.PhotoMediaID, val.PhotoMediaID)
	}
}

func TestBindingRules(t *testing.T) {
	httpx.ConfigureValidator()
	blank, empty, longPhone, ok := "   ", "", "123", "Budi"
	cases := []struct {
		name string
		v    any
		ok   bool
	}{
		{"patch empty body", &UpdateProfileRequest{}, true},
		{"patch valid name", &UpdateProfileRequest{Name: &ok}, true},
		{"patch blank name", &UpdateProfileRequest{Name: &blank}, false},
		{"patch empty name", &UpdateProfileRequest{Name: &empty}, false},
		{"patch short phone", &UpdateProfileRequest{Phone: &longPhone}, false},
		{"register +62 phone", &RegisterRequest{Name: "A", Email: "a@b.co", Phone: "+6281234567890", Password: "Password123!", AccountType: "buyer"}, false},
		{"register bad account type", &RegisterRequest{Name: "A", Email: "a@b.co", Phone: "81234567890", Password: "Password123!", AccountType: "admin"}, false},
		{"register ok", &RegisterRequest{Name: "A", Email: "a@b.co", Phone: "81234567890", Password: "Password123!", AccountType: "merchant"}, true},
		{"password same as current", &ChangePasswordRequest{Current: "Password123!", New: "Password123!"}, false},
		{"password different", &ChangePasswordRequest{Current: "Password123!", New: "PasswordBaru1"}, true},
	}
	for _, c := range cases {
		if err := binding.Validator.ValidateStruct(c.v); (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}
