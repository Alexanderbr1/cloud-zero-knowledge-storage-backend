package v1

import (
	"errors"
	"net/http"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	authuc "cloud-backend/internal/usecase/auth"
)

func registerAuth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.RegisterRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if !restapi.ValidateStruct(w, &in) {
			return
		}
		out, err := d.Auth.Register(r.Context(), in.Email, in.Password)
		if mapAuthErr(w, err) {
			return
		}
		writeTokenResponse(w, http.StatusCreated, out)
	}
}

func loginAuth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.LoginRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if !restapi.ValidateStruct(w, &in) {
			return
		}
		out, err := d.Auth.Login(r.Context(), in.Email, in.Password)
		if mapAuthErr(w, err) {
			return
		}
		writeTokenResponse(w, http.StatusOK, out)
	}
}

func refreshAuth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.RefreshRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if !restapi.ValidateStruct(w, &in) {
			return
		}
		out, err := d.Auth.Refresh(r.Context(), in.RefreshToken)
		if mapAuthErr(w, err) {
			return
		}
		writeTokenResponse(w, http.StatusOK, out)
	}
}

func logoutAuth(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in dto.LogoutRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !restapi.ValidateStruct(w, &in) {
			return
		}
		if err := d.Auth.Logout(r.Context(), in.RefreshToken); err != nil {
			mapAuthErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeTokenResponse(w http.ResponseWriter, status int, out authuc.TokenPair) {
	restapi.WriteJSON(w, status, dto.TokenResponse{
		AccessToken:      out.AccessToken,
		RefreshToken:     out.RefreshToken,
		ExpiresIn:        out.AccessExpiresIn,
		RefreshExpiresIn: out.RefreshExpiresIn,
		TokenType:        "Bearer",
	})
}

func mapAuthErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
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
	return true
}
