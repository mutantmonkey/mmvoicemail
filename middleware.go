package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

func getRequestSignature(baseUrl string, req *http.Request, authToken string) (sig []byte, err error) {
	var buf strings.Builder
	buf.WriteString(baseUrl)

	if req.Method == "POST" {
		// Sort all POST fields alphabetically by key and concatenate
		// the parameter name and value to the end of the URL (with no
		// delimiter)
		err = req.ParseForm()
		if err != nil {
			return
		}
		keys := make([]string, 0, len(req.PostForm))
		for k := range req.PostForm {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			buf.WriteString(k)
			buf.WriteString(req.PostForm.Get(k))
		}
	}

	mac := hmac.New(sha1.New, []byte(authToken))
	mac.Write([]byte(buf.String()))
	sig = mac.Sum(nil)
	return
}

func checkRequestSignature(u *url.URL, req *http.Request, authToken string) (bool, error) {
	// https://www.twilio.com/docs/usage/security#validating-requests

	// automatically add default port, if it is missing
	if u.Port() == "" {
		if u.Scheme == "https" {
			u.Host = fmt.Sprintf("%s:443", u.Host)
		} else {
			u.Host = fmt.Sprintf("%s:80", u.Host)
		}
	}

	sigWithPort, err := getRequestSignature(u.String(), req, authToken)
	if err != nil {
		return false, err
	}

	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		return false, err
	}
	u.Host = host
	sigWithoutPort, err := getRequestSignature(u.String(), req, authToken)
	if err != nil {
		return false, err
	}

	reqSignature, err := base64.StdEncoding.DecodeString(req.Header.Get("X-Twilio-Signature"))
	if err != nil {
		return false, err
	}

	return hmac.Equal(reqSignature, sigWithPort) || hmac.Equal(reqSignature, sigWithoutPort), nil
}

func TwilioValidatorMiddleware(authToken string, baseUrl *url.URL) func(http.Handler) http.Handler {
	f := func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, req *http.Request) {
			req.URL.Scheme = baseUrl.Scheme
			req.URL.Host = req.Host

			ok, err := checkRequestSignature(req.URL, req, authToken)
			if ok {
				h.ServeHTTP(w, req)
			} else {
				if err != nil {
					log.Printf("Error while checking request signature: %s\n", err)
				}
				http.Error(w, "403 forbidden", http.StatusForbidden)
			}
		}
		return http.HandlerFunc(fn)
	}
	return f
}

func ProxyFixMiddleware(numProxies int) func(http.Handler) http.Handler {
	f := func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, req *http.Request) {
			xff := strings.Split(req.Header.Get("X-Forwarded-For"), ", ")
			if len(xff) >= numProxies && xff[numProxies-1] != "" {
				req.RemoteAddr = xff[numProxies-1]
			} else {
				log.Print("Warning: unable to determine IP address from X-Forwarded-For. Ensure that PROXY_FIX and PROXY_FIX_NUM_PROXIES are configured correctly for your environment.")
			}

			h.ServeHTTP(w, req)
		}
		return http.HandlerFunc(fn)
	}
	return f
}
