package httpcontrol

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/wwwcont/ryazanvpn/internal/domain/invitecode"
)

type fakeInviteCodeRepo struct{}

func (f *fakeInviteCodeRepo) Create(ctx context.Context, in invitecode.CreateParams) (*invitecode.InviteCode, error) {
	return &invitecode.InviteCode{ID: "id-1", Code: in.Code, Status: in.Status, MaxActivations: in.MaxActivations}, nil
}

func (f *fakeInviteCodeRepo) RevokeByID(ctx context.Context, id string) (*invitecode.InviteCode, error) {
	return &invitecode.InviteCode{ID: id, Status: invitecode.CodeStatusRevoked}, nil
}

func TestGenerateInviteCodeFourDigits(t *testing.T) {
	re := regexp.MustCompile(`^\d{4}$`)
	for i := 0; i < 100; i++ {
		if code := generateInviteCode(); !re.MatchString(code) {
			t.Fatalf("expected 4-digit code, got %q", code)
		}
	}
}

func TestAdminCreateInviteCodeRejectsNonFourDigits(t *testing.T) {
	r := NewRouter(Options{
		AdminSecret: "secret",
		InviteCodes: &fakeInviteCodeRepo{},
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/invite-codes", bytes.NewBufferString(`{"code":"12345"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Secret", "secret")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

