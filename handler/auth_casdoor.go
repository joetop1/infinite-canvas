package handler

import (
	"net/http"
	"net/url"

	"github.com/tigerowo/infinite-canvas/service"
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
	values := url.Values{"redirect": []string{redirect}}
	if err != nil {
		values.Set("error", err.Error())
		http.Redirect(w, r, "/login?"+values.Encode(), http.StatusFound)
		return
	}
	// Keep the browser's public origin; forwarded hosts may contain the container address.
	// Fragment is consumed and removed by the login page, keeping the session token out of access logs.
	http.Redirect(w, r, "/login?"+values.Encode()+"#token="+url.QueryEscape(session.Token), http.StatusFound)
}
