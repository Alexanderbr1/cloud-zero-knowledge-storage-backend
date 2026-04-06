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
		out, err := d.Auth.Register(r.Context(), in.Email, in.Password, cryptoSalt)
		if err != nil {
			writeAuthErr(w, err)
			return
		}
		writeTokenResponse(w, d.RefreshCookie, http.StatusCreated, out)
	}
}

func login(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.LoginRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		out, err := d.Auth.Login(r.Context(), in.Email, in.Password)
		if err != nil {
			writeAuthErr(w, err)
			return
		}
		writeTokenResponse(w, d.RefreshCookie, http.StatusOK, out)
	}
}

func refresh(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rt := readRefreshToken(r, d.RefreshCookie.Name)
		if rt == "" {
			restapi.WriteError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}

		out, err := d.Auth.Refresh(r.Context(), rt)
		if err != nil {
			if errors.Is(err, authuc.ErrInvalidRefresh) {
				clearRefreshTokenCookie(w, d.RefreshCookie)
			}
			writeAuthErr(w, err)
			return
		}
		writeTokenResponse(w, d.RefreshCookie, http.StatusOK, out)
	}
}

func logout(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rt := readRefreshToken(r, d.RefreshCookie.Name)
		_ = d.Auth.Logout(r.Context(), rt)
		clearRefreshTokenCookie(w, d.RefreshCookie)
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeTokenResponse(w http.ResponseWriter, cookieCfg config.RefreshCookieConfig, status int, out authuc.TokenPair) {
	maxAge := int(out.RefreshExpiresIn)
	if maxAge < 0 {
		maxAge = 0
	}
	setRefreshTokenCookie(w, cookieCfg, out.RefreshToken, maxAge)

	body := dto.TokenResponse{
		AccessToken:      out.AccessToken,
		ExpiresIn:        out.AccessExpiresIn,
		RefreshExpiresIn: out.RefreshExpiresIn,
		TokenType:        tokenTypeBearer,
	}
	if len(out.CryptoSalt) > 0 {
		body.CryptoSalt = base64.StdEncoding.EncodeToString(out.CryptoSalt)
	}
	restapi.WriteJSON(w, status, body)
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
