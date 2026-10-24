package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tigerowo/infinite-canvas/config"
	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/repository"
	"golang.org/x/oauth2"
)

const casdoorCookie = "canvas_casdoor_login"

type casdoorLoginState struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Redirect string `json:"redirect"`
	Issuer   string `json:"issuer"`
	ClientID string `json:"clientId"`
	Callback string `json:"callback"`
	jwt.RegisteredClaims
}

func casdoorURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return nil, safeMessageError{message: "Casdoor 地址必须为 HTTPS；仅本机调试允许 HTTP"}
	}
	return u, nil
}

func validateCasdoorSetting(s model.PrivateCasdoorAuthSetting) error {
	if _, err := casdoorURL(s.Issuer); err != nil {
		return err
	}
	callback, err := casdoorURL(s.RedirectURI)
	if err != nil {
		return err
	}
	if callback.Path != "/api/auth/casdoor/callback" || strings.TrimSpace(s.ClientID) == "" || strings.TrimSpace(s.ClientSecret) == "" {
		return safeMessageError{message: "请填写 Casdoor Client ID、密钥及完整回调地址（/api/auth/casdoor/callback）"}
	}
	return nil
}

func casdoorSettings() (model.Settings, error) {
	s, err := repository.GetSettings()
	if err != nil {
		return s, err
	}
	if !s.Public.Auth.Casdoor.Enabled {
		return s, safeMessageError{message: "Casdoor 登录未开启"}
	}
	return s, validateCasdoorSetting(s.Private.Auth.Casdoor)
}

func casdoorProvider(ctx context.Context, s model.PrivateCasdoorAuthSetting) (*oidc.Provider, oauth2.Config, error) {
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: 15 * time.Second})
	provider, err := oidc.NewProvider(ctx, strings.TrimRight(strings.TrimSpace(s.Issuer), "/"))
	if err != nil {
		return nil, oauth2.Config{}, safeMessageError{message: "无法读取 Casdoor OIDC 配置，请检查服务地址和证书"}
	}
	var metadata struct {
		JWKS string `json:"jwks_uri"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, oauth2.Config{}, err
	}
	if _, err := casdoorURL(metadata.JWKS); err != nil {
		return nil, oauth2.Config{}, err
	}
	endpoint := provider.Endpoint()
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL} {
		if _, err := casdoorURL(raw); err != nil {
			return nil, oauth2.Config{}, err
		}
	}
	return provider, oauth2.Config{ClientID: strings.TrimSpace(s.ClientID), ClientSecret: s.ClientSecret, Endpoint: endpoint,
		RedirectURL: strings.TrimSpace(s.RedirectURI), Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}, nil
}

func ClearCasdoorLogin(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: casdoorCookie, Value: "", Path: "/api/auth/casdoor", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func CasdoorAuthorizeURL(w http.ResponseWriter, r *http.Request, redirect string) (string, error) {
	settings, err := casdoorSettings()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	_, client, err := casdoorProvider(ctx, settings.Private.Auth.Casdoor)
	if err != nil {
		return "", err
	}
	target := safeRedirectPath(redirect)
	if len(target) > 1024 {
		target = "/"
	}
	s := casdoorLoginState{State: oauth2.GenerateVerifier(), Nonce: oauth2.GenerateVerifier(), Verifier: oauth2.GenerateVerifier(),
		Redirect: target, Issuer: settings.Private.Auth.Casdoor.Issuer, ClientID: client.ClientID, Callback: client.RedirectURL,
		RegisteredClaims: jwt.RegisteredClaims{Subject: "casdoor-login", ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute))}}
	cookie, err := jwt.NewWithClaims(jwt.SigningMethodHS256, s).SignedString([]byte(config.Cfg.JWTSecret))
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{Name: casdoorCookie, Value: cookie, Path: "/api/auth/casdoor", HttpOnly: true,
		Secure: strings.HasPrefix(client.RedirectURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	return client.AuthCodeURL(s.State, oidc.Nonce(s.Nonce), oauth2.S256ChallengeOption(s.Verifier)), nil
}

func LoginWithCasdoor(r *http.Request) (model.AuthSession, string, error) {
	invalid := safeMessageError{message: "Casdoor 登录已过期或校验失败，请重新登录"}
	cookie, err := r.Cookie(casdoorCookie)
	if err != nil {
		return model.AuthSession{}, "/", invalid
	}
	var state casdoorLoginState
	token, err := jwt.ParseWithClaims(cookie.Value, &state, func(t *jwt.Token) (any, error) { return []byte(config.Cfg.JWTSecret), nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithSubject("casdoor-login"))
	if err != nil || !token.Valid || state.State == "" || subtle.ConstantTimeCompare([]byte(state.State), []byte(r.URL.Query().Get("state"))) != 1 {
		return model.AuthSession{}, "/", invalid
	}
	redirect := safeRedirectPath(state.Redirect)
	settings, err := casdoorSettings()
	if err != nil {
		return model.AuthSession{}, redirect, err
	}
	s := settings.Private.Auth.Casdoor
	if s.Issuer != state.Issuer || strings.TrimSpace(s.ClientID) != state.ClientID || strings.TrimSpace(s.RedirectURI) != state.Callback {
		return model.AuthSession{}, redirect, invalid
	}
	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		return model.AuthSession{}, redirect, invalid
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: 15 * time.Second})
	provider, client, err := casdoorProvider(ctx, s)
	if err != nil {
		return model.AuthSession{}, redirect, err
	}
	exchanged, err := client.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(state.Verifier))
	if err != nil {
		return model.AuthSession{}, redirect, safeMessageError{message: "Casdoor 授权码兑换失败，请检查应用配置后重新登录"}
	}
	raw, ok := exchanged.Extra("id_token").(string)
	if !ok {
		return model.AuthSession{}, redirect, invalid
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: client.ClientID}).Verify(ctx, raw)
	if err != nil || idToken.Subject == "" || idToken.Nonce != state.Nonce {
		return model.AuthSession{}, redirect, invalid
	}
	var profile struct {
		Name          string `json:"name"`
		Username      string `json:"preferred_username"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&profile); err != nil {
		return model.AuthSession{}, redirect, invalid
	}
	key := sha256.Sum256([]byte(idToken.Issuer + "\x00" + idToken.Subject))
	identity := hex.EncodeToString(key[:])
	id := "casdoor-" + identity
	existing, found, err := repository.GetUserByID(id)
	if err != nil {
		return model.AuthSession{}, redirect, err
	}
	if found && existing.CasdoorID != identity {
		return model.AuthSession{}, redirect, errors.New("Casdoor 账号绑定不一致")
	}
	if !found && settings.Public.Auth.AllowRegister != nil && !*settings.Public.Auth.AllowRegister {
		return model.AuthSession{}, redirect, safeMessageError{message: "当前未开放注册"}
	}
	email := ""
	if profile.EmailVerified {
		email = strings.TrimSpace(profile.Email)
	}
	user, err := repository.SaveCasdoorLogin(model.User{ID: id, CasdoorID: identity, Username: "casdoor_" + identity[:24],
		DisplayName: firstNonEmpty(profile.Name, profile.Username, "Casdoor 用户"), Email: email, Role: model.UserRoleUser,
		Status: model.UserStatusActive, AffCode: newAffCode(), CreatedAt: now()}, !found)
	if err != nil {
		return model.AuthSession{}, redirect, err
	}
	if user.Status != model.UserStatusActive {
		return model.AuthSession{}, redirect, safeMessageError{message: "账号已被禁用"}
	}
	session, err := newSession(user)
	return session, redirect, err
}
