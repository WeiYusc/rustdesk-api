package service

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"gorm.io/gorm"
)

const (
	passkeyChallengeTTL           = 5 * time.Minute
	maxActiveLoginChallengesPerIP = 20
)

var passkeyVerify passkeyVerifier = defaultPasskeyVerifier{}

type PasskeyService struct{}

type PublicKeyCredentialRpEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PublicKeyCredentialUserEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

type PublicKeyCredentialParameters struct {
	Type string `json:"type"`
	Alg  int    `json:"alg"`
}

type AuthenticatorSelectionCriteria struct {
	ResidentKey        string `json:"residentKey"`
	RequireResidentKey bool   `json:"requireResidentKey"`
	UserVerification   string `json:"userVerification"`
}

type PublicKeyCredentialDescriptor struct {
	Type       string   `json:"type"`
	ID         string   `json:"id"`
	Transports []string `json:"transports,omitempty"`
}

type PublicKeyCredentialCreationOptions struct {
	Challenge              string                          `json:"challenge"`
	RP                     PublicKeyCredentialRpEntity     `json:"rp"`
	User                   PublicKeyCredentialUserEntity   `json:"user"`
	PubKeyCredParams       []PublicKeyCredentialParameters `json:"pubKeyCredParams"`
	Timeout                int                             `json:"timeout"`
	Attestation            string                          `json:"attestation"`
	AuthenticatorSelection AuthenticatorSelectionCriteria  `json:"authenticatorSelection"`
	ExcludeCredentials     []PublicKeyCredentialDescriptor `json:"excludeCredentials,omitempty"`
}

type PublicKeyCredentialRequestOptions struct {
	Challenge        string                          `json:"challenge"`
	RPID             string                          `json:"rpId"`
	Timeout          int                             `json:"timeout"`
	UserVerification string                          `json:"userVerification"`
	AllowCredentials []PublicKeyCredentialDescriptor `json:"allowCredentials,omitempty"`
}

type passkeyChallengeData struct {
	RPID           string               `json:"rp_id"`
	Origin         string               `json:"origin"`
	AllowedOrigins []string             `json:"allowed_origins"`
	Session        webauthn.SessionData `json:"session"`
}

type passkeyVerifier interface {
	FinishRegistration(user *model.User, challenge *model.AuthChallenge, payload []byte) (*webauthn.Credential, error)
	FinishLogin(challenge *model.AuthChallenge, payload []byte) (*model.User, *webauthn.Credential, error)
}

type defaultPasskeyVerifier struct{}

type passkeyWebAuthnUser struct {
	user        *model.User
	credentials []webauthn.Credential
}

func (u passkeyWebAuthnUser) WebAuthnID() []byte {
	return decodeB64URLOrBytes(u.user.WebauthnUserHandle)
}

func (u passkeyWebAuthnUser) WebAuthnName() string {
	return u.user.Username
}

func (u passkeyWebAuthnUser) WebAuthnDisplayName() string {
	return u.user.Username
}

func (u passkeyWebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

type PasskeyItem struct {
	ID             uint       `json:"id"`
	Name           string     `json:"name"`
	CredentialID   string     `json:"credential_id"`
	AAGUID         string     `json:"aaguid,omitempty"`
	Transports     []string   `json:"transports"`
	BackupEligible bool       `json:"backup_eligible"`
	BackupState    bool       `json:"backup_state"`
	CloneWarning   bool       `json:"clone_warning"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	CreatedAt      any        `json:"created_at"`
	UpdatedAt      any        `json:"updated_at"`
}

func (s *PasskeyService) List(userID uint) ([]PasskeyItem, error) {
	var passkeys []model.UserPasskey
	if err := global.DB.Where("user_id = ?", userID).Order("created_at desc").Find(&passkeys).Error; err != nil {
		return nil, err
	}
	items := make([]PasskeyItem, 0, len(passkeys))
	for _, passkey := range passkeys {
		items = append(items, PasskeyItem{
			ID:             passkey.Id,
			Name:           passkey.Name,
			CredentialID:   passkey.CredentialID,
			AAGUID:         passkey.AAGUID,
			Transports:     splitTransports(passkey.Transports),
			BackupEligible: passkey.BackupEligible,
			BackupState:    passkey.BackupState,
			CloneWarning:   passkey.CloneWarning,
			LastUsedAt:     passkey.LastUsedAt,
			CreatedAt:      passkey.CreatedAt,
			UpdatedAt:      passkey.UpdatedAt,
		})
	}
	return items, nil
}

func (s *PasskeyService) Rename(userID uint, passkeyID uint, name string) error {
	name, err := normalizePasskeyName(name)
	if err != nil || passkeyID == 0 {
		return fmt.Errorf("ParamsError")
	}
	var passkey model.UserPasskey
	if err := global.DB.Where("id = ? and user_id = ?", passkeyID, userID).First(&passkey).Error; err != nil {
		return fmt.Errorf("PasskeyNotFound")
	}
	if passkey.Name == name {
		return nil
	}
	return global.DB.Model(&passkey).Update("name", name).Error
}

func (s *PasskeyService) Delete(userID uint, passkeyID uint) error {
	if passkeyID == 0 {
		return fmt.Errorf("ParamsError")
	}
	res := global.DB.Where("id = ? and user_id = ?", passkeyID, userID).Delete(&model.UserPasskey{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("PasskeyNotFound")
	}
	return nil
}

func normalizePasskeyName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 128 {
		return "", fmt.Errorf("ParamsError")
	}
	return name, nil
}

func (s *PasskeyService) BeginRegistration(user *model.User, ip string) (*PublicKeyCredentialCreationOptions, error) {
	if user == nil || user.Id == 0 {
		return nil, fmt.Errorf("invalid user")
	}
	settings, err := AllService.SettingsService.GetPasskey()
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return nil, fmt.Errorf("PasskeyDisabled")
	}
	if err := settings.validate(); err != nil {
		return nil, err
	}
	if _, err := ensureWebauthnUserHandle(user); err != nil {
		return nil, err
	}
	credentials, err := webauthnCredentialsForUser(user.Id)
	if err != nil {
		return nil, err
	}
	wa, err := newWebAuthn(settings)
	if err != nil {
		return nil, err
	}
	creation, session, err := wa.BeginMediatedRegistration(
		passkeyWebAuthnUser{user: user, credentials: credentials},
		protocol.MediationDefault,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirement(settings.ResidentKeyRequirement)),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
		webauthn.WithExclusions(webauthn.Credentials(credentials).CredentialDescriptors()),
		webauthn.WithExtensions(map[string]any{"credProps": true}),
	)
	if err != nil {
		return nil, err
	}
	if err := storePasskeyChallenge(session.Challenge, user.Id, model.AuthChallengeTypePasskeyRegister, settings, ip, *session); err != nil {
		return nil, err
	}
	return creationOptionsFromWebAuthn(creation), nil
}

func (s *PasskeyService) BeginLogin(ip string) (*PublicKeyCredentialRequestOptions, error) {
	settings, err := AllService.SettingsService.GetPasskey()
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return nil, fmt.Errorf("PasskeyDisabled")
	}
	if !settings.DiscoverableLoginEnabled {
		return nil, fmt.Errorf("PasskeyDiscoverableLoginDisabled")
	}
	if err := settings.validate(); err != nil {
		return nil, err
	}
	if err := enforceActiveLoginChallengeLimit(ip); err != nil {
		return nil, err
	}
	wa, err := newWebAuthn(settings)
	if err != nil {
		return nil, err
	}
	assertion, session, err := wa.BeginDiscoverableMediatedLogin(
		protocol.MediationDefault,
		webauthn.WithUserVerification(protocol.UserVerificationRequirement(settings.UserVerification)),
	)
	if err != nil {
		return nil, err
	}
	if err := storePasskeyChallenge(session.Challenge, 0, model.AuthChallengeTypePasskeyLogin, settings, ip, *session); err != nil {
		return nil, err
	}
	return requestOptionsFromWebAuthn(assertion), nil
}

func (s *PasskeyService) FinishRegistration(user *model.User, name string, payload []byte, ip string) error {
	if user == nil || user.Id == 0 {
		return fmt.Errorf("PasskeyVerificationFailed")
	}
	challengeID, err := challengeFromPayload(payload)
	if err != nil {
		return fmt.Errorf("PasskeyVerificationFailed")
	}
	challenge, err := loadActivePasskeyChallenge(challengeID, user.Id, model.AuthChallengeTypePasskeyRegister)
	if err != nil {
		return fmt.Errorf("PasskeyVerificationFailed")
	}
	credential, err := passkeyVerify.FinishRegistration(user, challenge, payload)
	if err != nil {
		return fmt.Errorf("PasskeyVerificationFailed")
	}
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := markChallengeUsed(tx, challenge.Id); err != nil {
			return err
		}
		return persistPasskeyCredential(tx, user, name, credential)
	}); err != nil {
		return fmt.Errorf("PasskeyVerificationFailed")
	}
	return nil
}

func (s *PasskeyService) FinishLogin(payload []byte, ip string) (*model.User, string, error) {
	challengeID, err := challengeFromPayload(payload)
	if err != nil {
		return nil, "", fmt.Errorf("PasskeyVerificationFailed")
	}
	challenge, err := loadActivePasskeyChallenge(challengeID, 0, model.AuthChallengeTypePasskeyLogin)
	if err != nil {
		return nil, "", fmt.Errorf("PasskeyVerificationFailed")
	}
	user, credential, err := passkeyVerify.FinishLogin(challenge, payload)
	if err != nil || user == nil || user.Id == 0 || credential == nil {
		return nil, "", fmt.Errorf("PasskeyVerificationFailed")
	}
	var token string
	if err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := markChallengeUsed(tx, challenge.Id); err != nil {
			return err
		}
		if err := updatePasskeyCredentialAfterLogin(tx, user.Id, credential); err != nil {
			return err
		}
		ut, err := createPasskeyLoginToken(tx, user, ip)
		if err != nil {
			return err
		}
		token = ut.Token
		return nil
	}); err != nil {
		return nil, "", fmt.Errorf("PasskeyVerificationFailed")
	}
	return user, token, nil
}

func (defaultPasskeyVerifier) FinishRegistration(user *model.User, challenge *model.AuthChallenge, payload []byte) (*webauthn.Credential, error) {
	data, err := parsePasskeyChallengeData(challenge)
	if err != nil {
		return nil, err
	}
	settings, err := AllService.SettingsService.GetPasskey()
	if err != nil {
		return nil, err
	}
	credentials, err := webauthnCredentialsForUser(user.Id)
	if err != nil {
		return nil, err
	}
	wa, err := newWebAuthn(settings)
	if err != nil {
		return nil, err
	}
	request := httptest.NewRequest("POST", "/api/admin/passkey/register/finish", bytes.NewReader(passkeyCredentialPayload(payload)))
	return wa.FinishRegistration(passkeyWebAuthnUser{user: user, credentials: credentials}, data.Session, request)
}

func (defaultPasskeyVerifier) FinishLogin(challenge *model.AuthChallenge, payload []byte) (*model.User, *webauthn.Credential, error) {
	data, err := parsePasskeyChallengeData(challenge)
	if err != nil {
		return nil, nil, err
	}
	settings, err := AllService.SettingsService.GetPasskey()
	if err != nil {
		return nil, nil, err
	}
	wa, err := newWebAuthn(settings)
	if err != nil {
		return nil, nil, err
	}
	request := httptest.NewRequest("POST", "/api/admin/passkey/login/finish", bytes.NewReader(passkeyCredentialPayload(payload)))
	validatedUser, credential, err := wa.FinishPasskeyLogin(loadDiscoverablePasskeyUser, data.Session, request)
	if err != nil {
		return nil, nil, err
	}
	adapter, ok := validatedUser.(passkeyWebAuthnUser)
	if !ok {
		return nil, nil, fmt.Errorf("unexpected passkey user adapter")
	}
	return adapter.user, credential, nil
}

func passkeyCredentialPayload(payload []byte) []byte {
	var request struct {
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || len(request.Credential) == 0 {
		return payload
	}
	return request.Credential
}

func challengeFromPayload(payload []byte) (string, error) {
	var request struct {
		ChallengeID string `json:"challenge_id"`
		Challenge   string `json:"challenge"`
		Credential  *struct {
			Response struct {
				ClientDataJSON string `json:"clientDataJSON"`
			} `json:"response"`
		} `json:"credential"`
		Response struct {
			ClientDataJSON string `json:"clientDataJSON"`
		} `json:"response"`
	}
	if err := json.Unmarshal(payload, &request); err != nil {
		return "", err
	}
	clientDataValue := request.Response.ClientDataJSON
	if clientDataValue == "" && request.Credential != nil {
		clientDataValue = request.Credential.Response.ClientDataJSON
	}
	if clientDataValue == "" {
		return "", fmt.Errorf("missing challenge or clientDataJSON")
	}
	clientDataJSON, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(clientDataValue, "="))
	if err != nil {
		return "", err
	}
	var clientData struct {
		Challenge string `json:"challenge"`
	}
	if err := json.Unmarshal(clientDataJSON, &clientData); err != nil {
		return "", err
	}
	if clientData.Challenge == "" {
		return "", fmt.Errorf("missing challenge")
	}
	if request.ChallengeID != "" && request.ChallengeID != clientData.Challenge {
		return "", fmt.Errorf("challenge mismatch")
	}
	return clientData.Challenge, nil
}

func loadActivePasskeyChallenge(challengeID string, userID uint, challengeType string) (*model.AuthChallenge, error) {
	var challenge model.AuthChallenge
	err := global.DB.Where("challenge_id = ? and user_id = ? and type = ? and used_at is null", challengeID, userID, challengeType).First(&challenge).Error
	if err != nil {
		return nil, err
	}
	if !challenge.ExpiresAt.After(time.Now()) {
		return nil, fmt.Errorf("challenge expired")
	}
	return &challenge, nil
}

func ensureWebauthnUserHandle(user *model.User) (string, error) {
	if user.WebauthnUserHandle != "" {
		return user.WebauthnUserHandle, nil
	}
	handle, err := randomB64URL(32)
	if err != nil {
		return "", err
	}
	if err := global.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("webauthn_user_handle", handle).Error; err != nil {
		return "", err
	}
	user.WebauthnUserHandle = handle
	return handle, nil
}

func existingCredentialDescriptors(userID uint) ([]PublicKeyCredentialDescriptor, error) {
	var passkeys []model.UserPasskey
	if err := global.DB.Where("user_id = ?", userID).Find(&passkeys).Error; err != nil {
		return nil, err
	}
	descriptors := make([]PublicKeyCredentialDescriptor, 0, len(passkeys))
	for _, passkey := range passkeys {
		descriptors = append(descriptors, PublicKeyCredentialDescriptor{Type: "public-key", ID: passkey.CredentialID, Transports: splitTransports(passkey.Transports)})
	}
	return descriptors, nil
}

func enforceActiveLoginChallengeLimit(ip string) error {
	if ip == "" {
		return nil
	}
	var count int64
	err := global.DB.Model(&model.AuthChallenge{}).
		Where("type = ? and user_id = ? and ip = ? and used_at is null and expires_at > ?", model.AuthChallengeTypePasskeyLogin, 0, ip, time.Now()).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count >= maxActiveLoginChallengesPerIP {
		return fmt.Errorf("PasskeyLoginRateLimited")
	}
	return nil
}

func storePasskeyChallenge(challenge string, userID uint, challengeType string, settings PasskeySettings, ip string, session webauthn.SessionData) error {
	data, err := json.Marshal(passkeyChallengeData{RPID: settings.RPID, Origin: firstAllowedOrigin(settings.AllowedOrigins), AllowedOrigins: settings.AllowedOrigins, Session: session})
	if err != nil {
		return err
	}
	now := time.Now()
	return global.DB.Transaction(func(tx *gorm.DB) error {
		if challengeType == model.AuthChallengeTypePasskeyRegister {
			if err := tx.Model(&model.AuthChallenge{}).
				Where("type = ? and user_id = ? and used_at is null", challengeType, userID).
				Update("used_at", now).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.AuthChallenge{
			ChallengeID: challenge,
			UserId:      userID,
			Type:        challengeType,
			Data:        string(data),
			ExpiresAt:   now.Add(passkeyChallengeTTL),
			Ip:          ip,
		}).Error
	})
}

func randomB64URL(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func firstAllowedOrigin(origins []string) string {
	if len(origins) == 0 {
		return ""
	}
	return origins[0]
}

func newWebAuthn(settings PasskeySettings) (*webauthn.WebAuthn, error) {
	return webauthn.New(&webauthn.Config{
		RPDisplayName: settings.RPName,
		RPID:          settings.RPID,
		RPOrigins:     settings.AllowedOrigins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirement(settings.ResidentKeyRequirement),
			UserVerification: protocol.UserVerificationRequirement(settings.UserVerification),
		},
	})
}

func creationOptionsFromWebAuthn(creation *protocol.CredentialCreation) *PublicKeyCredentialCreationOptions {
	response := creation.Response
	params := make([]PublicKeyCredentialParameters, 0, len(response.Parameters))
	for _, param := range response.Parameters {
		params = append(params, PublicKeyCredentialParameters{Type: string(param.Type), Alg: int(param.Algorithm)})
	}
	exclude := make([]PublicKeyCredentialDescriptor, 0, len(response.CredentialExcludeList))
	for _, descriptor := range response.CredentialExcludeList {
		exclude = append(exclude, PublicKeyCredentialDescriptor{Type: string(descriptor.Type), ID: descriptor.CredentialID.String(), Transports: protocolTransportsToStrings(descriptor.Transport)})
	}
	return &PublicKeyCredentialCreationOptions{
		Challenge: response.Challenge.String(),
		RP: PublicKeyCredentialRpEntity{
			ID:   response.RelyingParty.ID,
			Name: response.RelyingParty.Name,
		},
		User: PublicKeyCredentialUserEntity{
			ID:          fmt.Sprint(response.User.ID),
			Name:        response.User.Name,
			DisplayName: response.User.DisplayName,
		},
		PubKeyCredParams: params,
		Timeout:          response.Timeout,
		Attestation:      string(response.Attestation),
		AuthenticatorSelection: AuthenticatorSelectionCriteria{
			ResidentKey:        string(response.AuthenticatorSelection.ResidentKey),
			RequireResidentKey: response.AuthenticatorSelection.RequireResidentKey != nil && *response.AuthenticatorSelection.RequireResidentKey,
			UserVerification:   string(response.AuthenticatorSelection.UserVerification),
		},
		ExcludeCredentials: exclude,
	}
}

func requestOptionsFromWebAuthn(assertion *protocol.CredentialAssertion) *PublicKeyCredentialRequestOptions {
	response := assertion.Response
	allowCredentials := make([]PublicKeyCredentialDescriptor, 0, len(response.AllowedCredentials))
	for _, descriptor := range response.AllowedCredentials {
		allowCredentials = append(allowCredentials, PublicKeyCredentialDescriptor{Type: string(descriptor.Type), ID: descriptor.CredentialID.String(), Transports: protocolTransportsToStrings(descriptor.Transport)})
	}
	return &PublicKeyCredentialRequestOptions{
		Challenge:        response.Challenge.String(),
		RPID:             response.RelyingPartyID,
		Timeout:          response.Timeout,
		UserVerification: string(response.UserVerification),
		AllowCredentials: allowCredentials,
	}
}

func parsePasskeyChallengeData(challenge *model.AuthChallenge) (passkeyChallengeData, error) {
	var data passkeyChallengeData
	if challenge == nil {
		return data, fmt.Errorf("missing challenge")
	}
	if err := json.Unmarshal([]byte(challenge.Data), &data); err != nil {
		return data, err
	}
	if data.Session.Challenge == "" {
		return data, fmt.Errorf("missing webauthn session")
	}
	return data, nil
}

func webauthnCredentialsForUser(userID uint) ([]webauthn.Credential, error) {
	var passkeys []model.UserPasskey
	if err := global.DB.Where("user_id = ?", userID).Find(&passkeys).Error; err != nil {
		return nil, err
	}
	credentials := make([]webauthn.Credential, 0, len(passkeys))
	for _, passkey := range passkeys {
		credentials = append(credentials, webauthnCredentialFromModel(passkey))
	}
	return credentials, nil
}

func webauthnCredentialFromModel(passkey model.UserPasskey) webauthn.Credential {
	return webauthn.Credential{
		ID:              decodeB64URLOrBytes(passkey.CredentialID),
		PublicKey:       decodeB64URLOrBytes(passkey.PublicKey),
		AttestationType: passkey.AttestationType,
		Transport:       stringsToProtocolTransports(splitTransports(passkey.Transports)),
		Flags: webauthn.CredentialFlags{
			BackupEligible: passkey.BackupEligible,
			BackupState:    passkey.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:       decodeHexOrBytes(passkey.AAGUID),
			SignCount:    passkey.SignCount,
			CloneWarning: passkey.CloneWarning,
		},
	}
}

func persistPasskeyCredential(tx *gorm.DB, user *model.User, name string, credential *webauthn.Credential) error {
	if credential == nil || len(credential.ID) == 0 || len(credential.PublicKey) == 0 {
		return fmt.Errorf("invalid credential")
	}
	now := time.Now()
	passkeyName, err := normalizePasskeyName(name)
	if err != nil {
		passkeyName = "Passkey"
	}
	passkey := &model.UserPasskey{
		UserId:          user.Id,
		Name:            passkeyName,
		CredentialID:    base64.RawURLEncoding.EncodeToString(credential.ID),
		UserHandle:      user.WebauthnUserHandle,
		PublicKey:       base64.RawURLEncoding.EncodeToString(credential.PublicKey),
		AttestationType: credential.AttestationType,
		AAGUID:          hex.EncodeToString(credential.Authenticator.AAGUID),
		SignCount:       credential.Authenticator.SignCount,
		CloneWarning:    credential.Authenticator.CloneWarning,
		Transports:      strings.Join(protocolTransportsToStrings(credential.Transport), ","),
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
		LastUsedAt:      &now,
	}
	return tx.Create(passkey).Error
}

func updatePasskeyCredentialAfterLogin(tx *gorm.DB, userID uint, credential *webauthn.Credential) error {
	if credential == nil || len(credential.ID) == 0 {
		return fmt.Errorf("invalid credential")
	}
	now := time.Now()
	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	updates := map[string]any{
		"sign_count":      credential.Authenticator.SignCount,
		"clone_warning":   credential.Authenticator.CloneWarning,
		"backup_eligible": credential.Flags.BackupEligible,
		"backup_state":    credential.Flags.BackupState,
		"last_used_at":    &now,
	}
	res := tx.Model(&model.UserPasskey{}).Where("user_id = ? and credential_id = ?", userID, credentialID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("passkey credential not updated")
	}
	return nil
}

func markChallengeUsed(tx *gorm.DB, challengeID uint) error {
	now := time.Now()
	res := tx.Model(&model.AuthChallenge{}).Where("id = ? and used_at is null", challengeID).Update("used_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("challenge already used")
	}
	return nil
}

func createPasskeyLoginToken(tx *gorm.DB, user *model.User, ip string) (*model.UserToken, error) {
	if user == nil || user.Id == 0 {
		return nil, fmt.Errorf("invalid user")
	}
	token := AllService.UserService.GenerateToken(user)
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}
	ut := &model.UserToken{
		UserId:    user.Id,
		Token:     token,
		ExpiredAt: AllService.UserService.UserTokenExpireTimestamp(),
	}
	if err := tx.Create(ut).Error; err != nil {
		return nil, err
	}
	loginLog := &model.LoginLog{
		UserId:      user.Id,
		Client:      model.LoginLogClientWebAdmin,
		Ip:          ip,
		Type:        model.LoginLogTypeAccount,
		UserTokenId: ut.Id,
	}
	if err := tx.Create(loginLog).Error; err != nil {
		return nil, err
	}
	return ut, nil
}

func loadDiscoverablePasskeyUser(rawID, userHandle []byte) (webauthn.User, error) {
	credentialID := base64.RawURLEncoding.EncodeToString(rawID)
	handle := base64.RawURLEncoding.EncodeToString(userHandle)
	var passkey model.UserPasskey
	if err := global.DB.Where("credential_id = ? and user_handle = ?", credentialID, handle).First(&passkey).Error; err != nil {
		return nil, err
	}
	var user model.User
	if err := global.DB.Where("id = ? and status = ?", passkey.UserId, model.COMMON_STATUS_ENABLE).First(&user).Error; err != nil {
		return nil, err
	}
	credentials, err := webauthnCredentialsForUser(user.Id)
	if err != nil {
		return nil, err
	}
	return passkeyWebAuthnUser{user: &user, credentials: credentials}, nil
}

func decodeB64URLOrBytes(value string) []byte {
	if value == "" {
		return nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
	if err == nil {
		return decoded
	}
	return []byte(value)
}

func decodeHexOrBytes(value string) []byte {
	if value == "" {
		return nil
	}
	decoded, err := hex.DecodeString(value)
	if err == nil {
		return decoded
	}
	return []byte(value)
}

func splitTransports(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func protocolTransportsToStrings(transports []protocol.AuthenticatorTransport) []string {
	out := make([]string, 0, len(transports))
	for _, transport := range transports {
		if transport != "" {
			out = append(out, string(transport))
		}
	}
	return out
}

func stringsToProtocolTransports(transports []string) []protocol.AuthenticatorTransport {
	out := make([]protocol.AuthenticatorTransport, 0, len(transports))
	for _, transport := range transports {
		out = append(out, protocol.AuthenticatorTransport(transport))
	}
	return out
}
