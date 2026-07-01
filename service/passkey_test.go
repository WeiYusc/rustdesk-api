package service

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPasskeyServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserToken{}, &model.LoginLog{}, &model.UserPasskey{}, &model.AuthChallenge{}, &model.Setting{}); err != nil {
		t.Fatalf("migrate passkey models: %v", err)
	}
	global.DB = db
	DB = db
	Config = &global.Config
	Jwt = jwt.NewJwt("", 0)
	AllService = &Service{UserService: &UserService{}, SettingsService: &SettingsService{}, PasskeyService: &PasskeyService{}}
	if err := AllService.SettingsService.SavePasskey(PasskeySettings{
		Enabled:                  true,
		RPName:                   "RustDesk Test Admin",
		RPID:                     "rd.plumire.cyou",
		AllowedOrigins:           []string{"https://rd.plumire.cyou"},
		UserVerification:         UserVerificationPreferred,
		DiscoverableLoginEnabled: true,
		ResidentKeyRequirement:   ResidentKeyRequired,
	}, 1); err != nil {
		t.Fatalf("save passkey settings: %v", err)
	}
	return db
}

func decodeB64URLTest(t *testing.T, value string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("%q is not base64url: %v", value, err)
	}
	return b
}

func passkeyClientDataPayload(t *testing.T, challenge string) []byte {
	t.Helper()
	clientData, err := json.Marshal(map[string]string{"challenge": challenge})
	if err != nil {
		t.Fatalf("marshal client data: %v", err)
	}
	payload, err := json.Marshal(map[string]any{
		"response": map[string]string{"clientDataJSON": base64.RawURLEncoding.EncodeToString(clientData)},
	})
	if err != nil {
		t.Fatalf("marshal passkey payload: %v", err)
	}
	return payload
}

type stubPasskeyVerifier struct {
	registrationCredential *webauthn.Credential
	loginUser              *model.User
	loginCredential        *webauthn.Credential
	registrationErr        error
	loginErr               error
}

func (s stubPasskeyVerifier) FinishRegistration(user *model.User, challenge *model.AuthChallenge, payload []byte) (*webauthn.Credential, error) {
	if s.registrationErr != nil {
		return nil, s.registrationErr
	}
	return s.registrationCredential, nil
}

func (s stubPasskeyVerifier) FinishLogin(challenge *model.AuthChallenge, payload []byte) (*model.User, *webauthn.Credential, error) {
	if s.loginErr != nil {
		return nil, nil, s.loginErr
	}
	return s.loginUser, s.loginCredential, nil
}

func TestPasskeyServiceRegisterBeginCreatesResidentKeyOptionsAndStableUserHandle(t *testing.T) {
	db := setupPasskeyServiceTestDB(t)
	isAdmin := true
	user := &model.User{Username: "admin", Email: "admin@example.test", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	options, err := AllService.PasskeyService.BeginRegistration(user, "127.0.0.1")
	if err != nil {
		t.Fatalf("BeginRegistration error: %v", err)
	}
	if options.Challenge == "" || len(decodeB64URLTest(t, options.Challenge)) < 32 {
		t.Fatalf("challenge = %q, want >=32 random bytes", options.Challenge)
	}
	if options.RP.ID != "rd.plumire.cyou" || options.RP.Name != "RustDesk Test Admin" {
		t.Fatalf("rp = %#v", options.RP)
	}
	if options.User.Name != "admin" || options.User.DisplayName != "admin" {
		t.Fatalf("user = %#v", options.User)
	}
	if len(decodeB64URLTest(t, options.User.ID)) < 32 {
		t.Fatalf("user.id = %q, want stable random handle", options.User.ID)
	}
	if options.AuthenticatorSelection.ResidentKey != ResidentKeyRequired || !options.AuthenticatorSelection.RequireResidentKey {
		t.Fatalf("authenticator selection = %#v", options.AuthenticatorSelection)
	}
	if options.AuthenticatorSelection.UserVerification != UserVerificationPreferred {
		t.Fatalf("userVerification = %q", options.AuthenticatorSelection.UserVerification)
	}
	if len(options.PubKeyCredParams) == 0 {
		t.Fatalf("PubKeyCredParams empty")
	}

	var saved model.User
	if err := db.First(&saved, user.Id).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if saved.WebauthnUserHandle == "" || saved.WebauthnUserHandle != options.User.ID {
		t.Fatalf("saved handle = %q, options user id = %q", saved.WebauthnUserHandle, options.User.ID)
	}

	var challenge model.AuthChallenge
	if err := db.Where("challenge_id = ? and user_id = ? and type = ?", options.Challenge, user.Id, model.AuthChallengeTypePasskeyRegister).First(&challenge).Error; err != nil {
		t.Fatalf("find stored challenge: %v", err)
	}
	if time.Until(challenge.ExpiresAt) <= 0 {
		t.Fatalf("challenge already expired: %v", challenge.ExpiresAt)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(challenge.Data), &data); err != nil {
		t.Fatalf("challenge data json: %v", err)
	}
	if data["rp_id"] != "rd.plumire.cyou" || data["origin"] != "https://rd.plumire.cyou" {
		t.Fatalf("challenge data = %#v", data)
	}
	origins, ok := data["allowed_origins"].([]any)
	if !ok || len(origins) != 1 || origins[0] != "https://rd.plumire.cyou" {
		t.Fatalf("challenge allowed origins = %#v", data["allowed_origins"])
	}

	second, err := AllService.PasskeyService.BeginRegistration(user, "127.0.0.1")
	if err != nil {
		t.Fatalf("second BeginRegistration error: %v", err)
	}
	if second.User.ID != saved.WebauthnUserHandle {
		t.Fatalf("second user handle = %q, want %q", second.User.ID, saved.WebauthnUserHandle)
	}
}

func TestPasskeyServiceLoginBeginCreatesUsernameLessRequestOptions(t *testing.T) {
	db := setupPasskeyServiceTestDB(t)
	user := &model.User{Username: "admin", WebauthnUserHandle: "stable-handle"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.UserPasskey{UserId: user.Id, Name: "test key", CredentialID: "credential-id", UserHandle: "stable-handle", PublicKey: "public-key"}).Error; err != nil {
		t.Fatalf("create passkey: %v", err)
	}

	options, err := AllService.PasskeyService.BeginLogin("127.0.0.1")
	if err != nil {
		t.Fatalf("BeginLogin error: %v", err)
	}
	if options.RPID != "rd.plumire.cyou" || options.Challenge == "" {
		t.Fatalf("options = %#v", options)
	}
	if options.AllowCredentials != nil && len(options.AllowCredentials) != 0 {
		t.Fatalf("allowCredentials = %#v, want nil/empty for discoverable login", options.AllowCredentials)
	}
	if options.UserVerification != UserVerificationPreferred {
		t.Fatalf("userVerification = %q", options.UserVerification)
	}
	var challenge model.AuthChallenge
	if err := db.Where("challenge_id = ? and user_id = 0 and type = ?", options.Challenge, model.AuthChallengeTypePasskeyLogin).First(&challenge).Error; err != nil {
		t.Fatalf("find login challenge: %v", err)
	}
}

func TestPasskeyServiceLoginBeginDoesNotInvalidateExistingLoginChallenge(t *testing.T) {
	db := setupPasskeyServiceTestDB(t)
	first, err := AllService.PasskeyService.BeginLogin("127.0.0.1")
	if err != nil {
		t.Fatalf("first BeginLogin error: %v", err)
	}
	second, err := AllService.PasskeyService.BeginLogin("127.0.0.2")
	if err != nil {
		t.Fatalf("second BeginLogin error: %v", err)
	}
	if first.Challenge == second.Challenge {
		t.Fatalf("login challenges are identical: %q", first.Challenge)
	}
	var firstChallenge model.AuthChallenge
	if err := db.Where("challenge_id = ?", first.Challenge).First(&firstChallenge).Error; err != nil {
		t.Fatalf("reload first login challenge: %v", err)
	}
	if firstChallenge.UsedAt != nil {
		t.Fatalf("second public login begin invalidated first challenge at %v", firstChallenge.UsedAt)
	}
}

func TestPasskeyServiceBeginLoginLimitsActiveChallengesPerIP(t *testing.T) {
	db := setupPasskeyServiceTestDB(t)
	for i := 0; i < maxActiveLoginChallengesPerIP; i++ {
		if err := db.Create(&model.AuthChallenge{
			ChallengeID: "existing-login-challenge-" + string(rune('a'+i)),
			UserId:      0,
			Type:        model.AuthChallengeTypePasskeyLogin,
			Data:        "{}",
			ExpiresAt:   time.Now().Add(time.Minute),
			Ip:          "192.0.2.10",
		}).Error; err != nil {
			t.Fatalf("create existing login challenge: %v", err)
		}
	}
	if _, err := AllService.PasskeyService.BeginLogin("192.0.2.10"); err == nil {
		t.Fatalf("BeginLogin succeeded despite active challenge cap for IP")
	}
	if _, err := AllService.PasskeyService.BeginLogin("192.0.2.11"); err != nil {
		t.Fatalf("BeginLogin for a different IP error: %v", err)
	}
}

func TestPasskeyServiceFinishRegistrationRejectsInvalidPayloadWithoutConsumingChallenge(t *testing.T) {
	db := setupPasskeyServiceTestDB(t)
	isAdmin := true
	user := &model.User{Username: "admin", Email: "admin@example.test", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	options, err := AllService.PasskeyService.BeginRegistration(user, "127.0.0.1")
	if err != nil {
		t.Fatalf("BeginRegistration error: %v", err)
	}
	if err := AllService.PasskeyService.FinishRegistration(user, []byte(`{"challenge":"`+options.Challenge+`"}`), "127.0.0.1"); err == nil {
		t.Fatalf("FinishRegistration invalid payload succeeded")
	}
	var count int64
	if err := db.Model(&model.UserPasskey{}).Where("user_id = ?", user.Id).Count(&count).Error; err != nil {
		t.Fatalf("count passkeys: %v", err)
	}
	if count != 0 {
		t.Fatalf("created %d passkeys for invalid registration payload", count)
	}
	var challenge model.AuthChallenge
	if err := db.Where("challenge_id = ?", options.Challenge).First(&challenge).Error; err != nil {
		t.Fatalf("reload challenge: %v", err)
	}
	if challenge.UsedAt != nil {
		t.Fatalf("invalid registration consumed challenge at %v", challenge.UsedAt)
	}
}

func TestPasskeyServiceFinishLoginRejectsInvalidPayloadWithoutIssuingToken(t *testing.T) {
	db := setupPasskeyServiceTestDB(t)
	user := &model.User{Username: "admin", WebauthnUserHandle: "stable-handle", Status: model.COMMON_STATUS_ENABLE}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&model.UserPasskey{UserId: user.Id, Name: "test key", CredentialID: "credential-id", UserHandle: "stable-handle", PublicKey: "public-key"}).Error; err != nil {
		t.Fatalf("create passkey: %v", err)
	}
	options, err := AllService.PasskeyService.BeginLogin("127.0.0.1")
	if err != nil {
		t.Fatalf("BeginLogin error: %v", err)
	}
	if _, _, err := AllService.PasskeyService.FinishLogin([]byte(`{"challenge":"`+options.Challenge+`"}`), "127.0.0.1"); err == nil {
		t.Fatalf("FinishLogin invalid payload succeeded")
	}
	var challenge model.AuthChallenge
	if err := db.Where("challenge_id = ?", options.Challenge).First(&challenge).Error; err != nil {
		t.Fatalf("reload challenge: %v", err)
	}
	if challenge.UsedAt != nil {
		t.Fatalf("invalid login consumed challenge at %v", challenge.UsedAt)
	}
}

func TestPasskeyServiceFinishRegistrationPersistsVerifiedCredentialAndConsumesChallenge(t *testing.T) {
	db := setupPasskeyServiceTestDB(t)
	originalVerifier := passkeyVerify
	t.Cleanup(func() { passkeyVerify = originalVerifier })
	isAdmin := true
	user := &model.User{Username: "admin", Email: "admin@example.test", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	options, err := AllService.PasskeyService.BeginRegistration(user, "127.0.0.1")
	if err != nil {
		t.Fatalf("BeginRegistration error: %v", err)
	}
	passkeyVerify = stubPasskeyVerifier{registrationCredential: &webauthn.Credential{
		ID:              []byte("credential-id-1"),
		PublicKey:       []byte("public-key-cbor"),
		AttestationType: "none",
		Authenticator:   webauthn.Authenticator{AAGUID: []byte{1, 2, 3, 4}, SignCount: 7},
		Flags:           webauthn.CredentialFlags{BackupEligible: true, BackupState: true},
	}}
	payload := passkeyClientDataPayload(t, options.Challenge)
	if err := AllService.PasskeyService.FinishRegistration(user, payload, "127.0.0.1"); err != nil {
		t.Fatalf("FinishRegistration error: %v", err)
	}
	var saved model.UserPasskey
	if err := db.Where("user_id = ?", user.Id).First(&saved).Error; err != nil {
		t.Fatalf("reload saved passkey: %v", err)
	}
	if saved.CredentialID != base64.RawURLEncoding.EncodeToString([]byte("credential-id-1")) || saved.PublicKey == "" || saved.SignCount != 7 {
		t.Fatalf("saved passkey = %#v", saved)
	}
	if saved.UserHandle != user.WebauthnUserHandle || !saved.BackupEligible || !saved.BackupState {
		t.Fatalf("saved passkey metadata = %#v, user handle %q", saved, user.WebauthnUserHandle)
	}
	var challenge model.AuthChallenge
	if err := db.Where("challenge_id = ?", options.Challenge).First(&challenge).Error; err != nil {
		t.Fatalf("reload challenge: %v", err)
	}
	if challenge.UsedAt == nil {
		t.Fatalf("successful registration did not consume challenge")
	}
}

func TestPasskeyServiceFinishRegistrationRejectsReusedChallenge(t *testing.T) {
	db := setupPasskeyServiceTestDB(t)
	originalVerifier := passkeyVerify
	t.Cleanup(func() { passkeyVerify = originalVerifier })
	isAdmin := true
	user := &model.User{Username: "admin", Email: "admin@example.test", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &isAdmin}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	options, err := AllService.PasskeyService.BeginRegistration(user, "127.0.0.1")
	if err != nil {
		t.Fatalf("BeginRegistration error: %v", err)
	}
	passkeyVerify = stubPasskeyVerifier{registrationCredential: &webauthn.Credential{
		ID:        []byte("credential-id-reuse"),
		PublicKey: []byte("public-key-cbor"),
	}}
	payload := passkeyClientDataPayload(t, options.Challenge)
	if err := AllService.PasskeyService.FinishRegistration(user, payload, "127.0.0.1"); err != nil {
		t.Fatalf("first FinishRegistration error: %v", err)
	}
	passkeyVerify = stubPasskeyVerifier{registrationCredential: &webauthn.Credential{
		ID:        []byte("credential-id-reuse-second"),
		PublicKey: []byte("public-key-cbor"),
	}}
	if err := AllService.PasskeyService.FinishRegistration(user, payload, "127.0.0.1"); err == nil {
		t.Fatalf("second FinishRegistration with reused challenge succeeded")
	}
	var count int64
	if err := db.Model(&model.UserPasskey{}).Where("user_id = ?", user.Id).Count(&count).Error; err != nil {
		t.Fatalf("count passkeys: %v", err)
	}
	if count != 1 {
		t.Fatalf("passkey count after reused challenge = %d, want 1", count)
	}
}

func TestPasskeyServiceFinishLoginRejectsReusedChallengeWithoutSecondToken(t *testing.T) {
	db := setupPasskeyServiceTestDB(t)
	originalVerifier := passkeyVerify
	t.Cleanup(func() { passkeyVerify = originalVerifier })
	user := &model.User{Username: "admin", WebauthnUserHandle: base64.RawURLEncoding.EncodeToString([]byte("stable-handle")), Status: model.COMMON_STATUS_ENABLE}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	credentialID := base64.RawURLEncoding.EncodeToString([]byte("credential-id-1"))
	if err := db.Create(&model.UserPasskey{UserId: user.Id, Name: "test key", CredentialID: credentialID, UserHandle: user.WebauthnUserHandle, PublicKey: base64.RawURLEncoding.EncodeToString([]byte("public-key")), SignCount: 1}).Error; err != nil {
		t.Fatalf("create passkey: %v", err)
	}
	options, err := AllService.PasskeyService.BeginLogin("127.0.0.1")
	if err != nil {
		t.Fatalf("BeginLogin error: %v", err)
	}
	passkeyVerify = stubPasskeyVerifier{loginUser: user, loginCredential: &webauthn.Credential{
		ID:            []byte("credential-id-1"),
		Authenticator: webauthn.Authenticator{SignCount: 9},
	}}
	payload := passkeyClientDataPayload(t, options.Challenge)
	if _, _, err := AllService.PasskeyService.FinishLogin(payload, "127.0.0.1"); err != nil {
		t.Fatalf("first FinishLogin error: %v", err)
	}
	if _, _, err := AllService.PasskeyService.FinishLogin(payload, "127.0.0.1"); err == nil {
		t.Fatalf("second FinishLogin with reused challenge succeeded")
	}
	var tokenCount int64
	if err := db.Model(&model.UserToken{}).Where("user_id = ?", user.Id).Count(&tokenCount).Error; err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if tokenCount != 1 {
		t.Fatalf("token count after reused challenge = %d, want 1", tokenCount)
	}
}

func TestPasskeyServiceFinishLoginUpdatesCredentialAndIssuesToken(t *testing.T) {
	db := setupPasskeyServiceTestDB(t)
	originalVerifier := passkeyVerify
	t.Cleanup(func() { passkeyVerify = originalVerifier })
	user := &model.User{Username: "admin", WebauthnUserHandle: base64.RawURLEncoding.EncodeToString([]byte("stable-handle")), Status: model.COMMON_STATUS_ENABLE}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	credentialID := base64.RawURLEncoding.EncodeToString([]byte("credential-id-1"))
	if err := db.Create(&model.UserPasskey{UserId: user.Id, Name: "test key", CredentialID: credentialID, UserHandle: user.WebauthnUserHandle, PublicKey: base64.RawURLEncoding.EncodeToString([]byte("public-key")), SignCount: 1}).Error; err != nil {
		t.Fatalf("create passkey: %v", err)
	}
	options, err := AllService.PasskeyService.BeginLogin("127.0.0.1")
	if err != nil {
		t.Fatalf("BeginLogin error: %v", err)
	}
	passkeyVerify = stubPasskeyVerifier{loginUser: user, loginCredential: &webauthn.Credential{
		ID:            []byte("credential-id-1"),
		Authenticator: webauthn.Authenticator{SignCount: 9},
		Flags:         webauthn.CredentialFlags{BackupEligible: true},
	}}
	validatedUser, token, err := AllService.PasskeyService.FinishLogin(passkeyClientDataPayload(t, options.Challenge), "127.0.0.1")
	if err != nil {
		t.Fatalf("FinishLogin error: %v", err)
	}
	if validatedUser.Id != user.Id || token == "" {
		t.Fatalf("validatedUser=%#v token=%q", validatedUser, token)
	}
	var saved model.UserPasskey
	if err := db.Where("credential_id = ?", credentialID).First(&saved).Error; err != nil {
		t.Fatalf("reload passkey: %v", err)
	}
	if saved.SignCount != 9 || saved.LastUsedAt == nil || !saved.BackupEligible {
		t.Fatalf("updated passkey = %#v", saved)
	}
	var tokenCount int64
	if err := db.Model(&model.UserToken{}).Where("user_id = ? and token = ?", user.Id, token).Count(&tokenCount).Error; err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if tokenCount != 1 {
		t.Fatalf("tokenCount=%d", tokenCount)
	}
	var challenge model.AuthChallenge
	if err := db.Where("challenge_id = ?", options.Challenge).First(&challenge).Error; err != nil {
		t.Fatalf("reload challenge: %v", err)
	}
	if challenge.UsedAt == nil {
		t.Fatalf("successful login did not consume challenge")
	}
}
