package v1

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/rs/zerolog"

	"cloud-backend/config"
	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	authuc "cloud-backend/internal/usecase/auth"
)

type AuthService interface {
	Register(ctx context.Context, p authuc.RegisterParams) (authuc.TokenPair, error)
	LoginInit(ctx context.Context, email, aHex string) (authuc.LoginInitResult, error)
	LoginFinalize(ctx context.Context, p authuc.LoginFinalizeParams) (authuc.LoginFinalizeResult, error)
	Refresh(ctx context.Context, refreshToken string) (authuc.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
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
		pair, err := d.Auth.Register(r.Context(), authuc.RegisterParams{
			Email:               in.Email,
			SRPSalt:             in.SRPSalt,
			SRPVerifier:         in.SRPVerifier,
			BcryptSalt:          in.BcryptSalt,
			CryptoSalt:          cryptoSalt,
			PublicKey:           publicKey,
			EncryptedPrivateKey: encPrivKey,
			Device:              parseDeviceInfo(r),
		})
		if err != nil {
			writeAuthErr(w, err, d.Logger)
			return
		}
		writeTokenResponse(w, d.RefreshCookie, http.StatusCreated, pair, "", nil)
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
		result, err := d.Auth.LoginFinalize(r.Context(), authuc.LoginFinalizeParams{
			SessionID: in.SessionID,
			M1:        in.M1,
			Device:    parseDeviceInfo(r),
		})
		if err != nil {
			writeAuthErr(w, err, d.Logger)
			return
		}
		writeTokenResponse(w, d.RefreshCookie, http.StatusOK, result.Pair, result.M2, result.EncryptedPrivateKey)
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
		writeTokenResponse(w, d.RefreshCookie, http.StatusOK, pair, "", nil)
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

func writeTokenResponse(
	w http.ResponseWriter,
	cookieCfg config.RefreshCookieConfig,
	status int,
	pair authuc.TokenPair,
	m2 string,
	encPrivKey []byte,
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
