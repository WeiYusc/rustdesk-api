package api

import (
	"testing"

	apiResp "github.com/lejianwen/rustdesk-api/v2/http/response/api"
	"github.com/lejianwen/rustdesk-api/v2/model"
)

func TestUserPayloadMapsBackendDisabledStatusForRustDeskClient(t *testing.T) {
	isAdmin := false
	user := &model.User{
		Username: "disabled-user",
		Email:    "disabled-user@example.test",
		Status:   model.COMMON_STATUS_DISABLED,
		IsAdmin:  &isAdmin,
	}

	payload := (&apiResp.UserPayload{}).FromUser(user)

	if payload.Status != 0 {
		t.Fatalf("disabled user payload status = %d, want RustDesk client disabled status 0", payload.Status)
	}
}
