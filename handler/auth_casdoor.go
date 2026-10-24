package handler

import (
	"github.com/tigerowo/infinite-canvas/service"
	"net/http"
	"net/url"
)

func CasdoorAuthorize(w http.ResponseWriter, r *http.Request) {
	target, err := service.CasdoorAuthorizeURL(w, r, r.URL.Query().Get("redirect"))
	if err != nil {
		FailError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

func CasdoorCallback(w http.ResponseWriter, r *http.Request) {
	service.ClearCasdoorLogin(w)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	session, redirect, err := service.LoginWithCasdoor(r)
	if err != nil {
		http.Redirect(w, r, loginRedirect(r, redirect, "", err.Error()), http.StatusFound)
		return
	}
	// Fragment is consumed and removed by the login page, keeping the session token out of access logs.
	http.Redirect(w, r, loginRedirect(r, redirect, "", "")+"#token="+url.QueryEscape(session.Token), http.StatusFound)
}
