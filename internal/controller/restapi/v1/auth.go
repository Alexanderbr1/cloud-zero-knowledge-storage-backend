package v1

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/config"
	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
)

type AuthService interface {
	Register(ctx context.Context, p authuc.RegisterParams) (authuc.TokenPair, error)
	LoginInit(ctx context.Context, email, aHex string) (authuc.LoginInitResult, error)
	LoginFinalize(ctx context.Context, p authuc.LoginFinalizeParams) (authuc.LoginFinalizeResult, error)
	Refresh(ctx context.Context, refreshToken string) (authuc.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	GetCryptoSalt(ctx context.Context, userID uuid.UUID) (cryptoSaltB64 string, kekEncMaster []byte, err error)
	RequestPasswordReset(ctx context.Context, email string) error
	GetRecoveryData(ctx context.Context, token string) (authuc.RecoveryData, bool, error)
	ResetPassword(ctx context.Context, token string, p authuc.ResetPasswordParams) error
}

const tokenTypeBearer = "Bearer"

func register(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.RegisterRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		cryptoSalt, ok := mustDecodeB64(w, in.CryptoSalt, "crypto_salt", 1, 0)
		if !ok {
			return
		}
		publicKey, ok := mustDecodeB64(w, in.PublicKey, "public_key", ecSpkiLen, ecSpkiLen)
		if !ok {
			return
		}
		// Minimum: 40 (AES-KW wrapped KWK) + 12 (GCM IV) + 1 (plaintext) + 16 (GCM tag) = 69.
		encPrivKey, ok := mustDecodeB64(w, in.EncryptedPrivateKey, "encrypted_private_key", 69, 0)
		if !ok {
			return
		}
		kekMaster, ok := mustDecodeB64(w, in.KEKEncryptedMaster, "kek_encrypted_master", 40, 40)
		if !ok {
			return
		}
		kekRecovery, ok := mustDecodeB64(w, in.KEKEncryptedRecovery, "kek_encrypted_recovery", 40, 40)
		if !ok {
			return
		}
		recSalt, ok := mustDecodeB64(w, in.RecoverySalt, "recovery_salt", 1, 0)
		if !ok {
			return
		}
		pair, err := d.Auth.Register(r.Context(), authuc.RegisterParams{
			Email:                in.Email,
			SRPSalt:              in.SRPSalt,
			SRPVerifier:          in.SRPVerifier,
			BcryptSalt:           in.BcryptSalt,
			CryptoSalt:           cryptoSalt,
			PublicKey:            publicKey,
			EncryptedPrivateKey:  encPrivKey,
			KEKEncryptedMaster:   kekMaster,
			KEKEncryptedRecovery: kekRecovery,
			RecoverySalt:         recSalt,
			Device:               parseDeviceInfo(r),
		})
		if err != nil {
			writeAuthErr(w, err, d.Logger)
			return
		}
		writeTokenResponse(w, d.RefreshCookie, http.StatusCreated, pair, "", nil, nil)
	}
}

func loginInit(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.LoginInitRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		result, err := d.Auth.LoginInit(r.Context(), in.Email, in.A)
		if err != nil {
			writeAuthErr(w, err, d.Logger)
			return
		}
		restapi.WriteJSON(w, http.StatusOK, dto.LoginInitResponse{
			SessionID:  result.SessionID,
			SRPSalt:    result.SRPSalt,
			BcryptSalt: result.BcryptSalt,
			B:          result.B,
			CryptoSalt: base64.StdEncoding.EncodeToString(result.CryptoSalt),
		})
	}
}

func loginFinalize(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.LoginFinalizeRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		device := parseDeviceInfo(r)
		result, err := d.Auth.LoginFinalize(r.Context(), authuc.LoginFinalizeParams{
			SessionID: in.SessionID,
			M1:        in.M1,
			Device:    device,
		})
		if err != nil {
			writeAuthErr(w, err, d.Logger)
			return
		}
		d.Audit.LogAsync(r.Context(), auditEvent(result.UserID, entity.AuditLoginSuccess, device.IPAddress, device.UserAgent, nil, ""))
		writeTokenResponse(w, d.RefreshCookie, http.StatusOK, result.Pair, result.M2, result.EncryptedPrivateKey, result.KEKEncryptedMaster)
	}
}

func refresh(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rt := readRefreshToken(r, d.RefreshCookie.Name)
		if rt == "" {
			restapi.WriteError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		pair, err := d.Auth.Refresh(r.Context(), rt)
		if err != nil {
			if errors.Is(err, authuc.ErrInvalidRefresh) {
				clearRefreshTokenCookie(w, d.RefreshCookie)
			}
			writeAuthErr(w, err, d.Logger)
			return
		}
		writeTokenResponse(w, d.RefreshCookie, http.StatusOK, pair, "", nil, nil)
	}
}

func logout(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rt := readRefreshToken(r, d.RefreshCookie.Name)
		if err := d.Auth.Logout(r.Context(), rt); err != nil {
			d.Logger.Warn().Err(err).Msg("logout failed")
		}
		clearRefreshTokenCookie(w, d.RefreshCookie)
		w.WriteHeader(http.StatusNoContent)
	}
}

func getCryptoSalt(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		saltB64, kekEncMaster, err := d.Auth.GetCryptoSalt(r.Context(), userID)
		if err != nil {
			d.Logger.Error().Err(err).Msg("get crypto salt")
			restapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		restapi.WriteJSON(w, http.StatusOK, dto.CryptoSaltResponse{
			CryptoSalt:         saltB64,
			KEKEncryptedMaster: base64.StdEncoding.EncodeToString(kekEncMaster),
		})
	}
}

func requestPasswordReset(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.ResetPasswordRequestRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		if err := d.Auth.RequestPasswordReset(r.Context(), in.Email); err != nil {
			d.Logger.Error().Err(err).Msg("request password reset")
			restapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func getRecoveryData(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.RecoveryDataRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		data, ok, err := d.Auth.GetRecoveryData(r.Context(), in.Token)
		if err != nil {
			d.Logger.Error().Err(err).Msg("get recovery data")
			restapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !ok || len(data.KEKEncryptedRecovery) == 0 || len(data.RecoverySalt) == 0 {
			restapi.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		restapi.WriteJSON(w, http.StatusOK, dto.RecoveryDataResponse{
			KEKEncryptedRecovery: base64.StdEncoding.EncodeToString(data.KEKEncryptedRecovery),
			RecoverySalt:         base64.StdEncoding.EncodeToString(data.RecoverySalt),
		})
	}
}

func resetPasswordConfirm(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.ResetPasswordConfirmRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		cryptoSalt, ok := mustDecodeB64(w, in.CryptoSalt, "crypto_salt", 1, 0)
		if !ok {
			return
		}
		kekEncMaster, ok := mustDecodeB64(w, in.KEKEncryptedMaster, "kek_encrypted_master", 40, 40)
		if !ok {
			return
		}
		kekEncRecovery, ok := mustDecodeB64(w, in.KEKEncryptedRecovery, "kek_encrypted_recovery", 40, 40)
		if !ok {
			return
		}
		recSalt, ok := mustDecodeB64(w, in.RecoverySalt, "recovery_salt", 1, 0)
		if !ok {
			return
		}
		err := d.Auth.ResetPassword(r.Context(), in.Token, authuc.ResetPasswordParams{
			SRPSalt:              in.SRPSalt,
			SRPVerifier:          in.SRPVerifier,
			BcryptSalt:           in.BcryptSalt,
			CryptoSalt:           cryptoSalt,
			KEKEncryptedMaster:   kekEncMaster,
			KEKEncryptedRecovery: kekEncRecovery,
			RecoverySalt:         recSalt,
		})
		if err != nil {
			if errors.Is(err, authuc.ErrResetTokenInvalid) {
				restapi.WriteError(w, http.StatusUnprocessableEntity, "invalid or expired reset token")
				return
			}
			d.Logger.Error().Err(err).Msg("reset password confirm")
			restapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeTokenResponse(
	w http.ResponseWriter,
	cookieCfg config.RefreshCookieConfig,
	status int,
	pair authuc.TokenPair,
	m2 string,
	encPrivKey []byte,
	kekEncMaster []byte,
) {
	maxAge := int(pair.RefreshExpiresIn)
	if maxAge < 0 {
		maxAge = 0
	}
	setRefreshTokenCookie(w, cookieCfg, pair.RefreshToken, maxAge)

	resp := dto.TokenResponse{
		AccessToken:      pair.AccessToken,
		ExpiresIn:        pair.AccessExpiresIn,
		RefreshExpiresIn: pair.RefreshExpiresIn,
		TokenType:        tokenTypeBearer,
		M2:               m2,
	}
	if len(encPrivKey) > 0 {
		resp.EncryptedPrivateKey = base64.StdEncoding.EncodeToString(encPrivKey)
	}
	if len(kekEncMaster) > 0 {
		resp.KEKEncryptedMaster = base64.StdEncoding.EncodeToString(kekEncMaster)
	}
	if len(pair.ClientKey) > 0 {
		resp.ClientKey = base64.StdEncoding.EncodeToString(pair.ClientKey)
	}
	restapi.WriteJSON(w, status, resp)
}

func writeAuthErr(w http.ResponseWriter, err error, log zerolog.Logger) {
	switch {
	case errors.Is(err, authuc.ErrInvalidInput):
		restapi.WriteError(w, http.StatusBadRequest, "bad request")
	case errors.Is(err, authuc.ErrUserExists):
		restapi.WriteError(w, http.StatusConflict, "user already exists")
	case errors.Is(err, authuc.ErrInvalidCredentials):
		restapi.WriteError(w, http.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, authuc.ErrInvalidRefresh):
		restapi.WriteError(w, http.StatusUnauthorized, "invalid refresh token")
	default:
		log.Error().Err(err).Msg("auth handler error")
		restapi.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
