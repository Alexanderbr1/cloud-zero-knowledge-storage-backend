package v1

import (
	"encoding/base64"
	"errors"
	"net/http"

	"cloud-backend/config"
	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	authuc "cloud-backend/internal/usecase/auth"
)

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
		cryptoSalt, err := base64.StdEncoding.DecodeString(in.CryptoSalt)
		if err != nil || len(cryptoSalt) == 0 {
			restapi.WriteError(w, http.StatusBadRequest, "invalid crypto_salt")
			return
		}
		pair, err := d.Auth.Register(r.Context(), authuc.RegisterParams{
			Email: in.Email, SRPSalt: in.SRPSalt, SRPVerifier: in.SRPVerifier,
			BcryptSalt: in.BcryptSalt, CryptoSalt: cryptoSalt, Device: parseDeviceInfo(r),
		})
		if err != nil {
			writeAuthErr(w, err)
			return
		}
		writeTokenResponse(w, d.RefreshCookie, http.StatusCreated, pair, "")
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
			writeAuthErr(w, err)
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
			SessionID: in.SessionID, M1: in.M1, Device: parseDeviceInfo(r),
		})
		if err != nil {
			writeAuthErr(w, err)
			return
		}
		writeTokenResponse(w, d.RefreshCookie, http.StatusOK, result.Pair, result.M2)
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
			writeAuthErr(w, err)
			return
		}
		writeTokenResponse(w, d.RefreshCookie, http.StatusOK, pair, "")
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

func writeTokenResponse(w http.ResponseWriter, cookieCfg config.RefreshCookieConfig, status int, pair authuc.TokenPair, m2 string) {
	maxAge := int(pair.RefreshExpiresIn)
	if maxAge < 0 {
		maxAge = 0
	}
	setRefreshTokenCookie(w, cookieCfg, pair.RefreshToken, maxAge)

	restapi.WriteJSON(w, status, dto.TokenResponse{
		AccessToken:      pair.AccessToken,
		ExpiresIn:        pair.AccessExpiresIn,
		RefreshExpiresIn: pair.RefreshExpiresIn,
		TokenType:        tokenTypeBearer,
		M2:               m2,
	})
}

func writeAuthErr(w http.ResponseWriter, err error) {
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
		restapi.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
