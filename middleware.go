package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
)

func checkRequestSignature(baseUrl string, req *http.Request, authToken string) (bool, error) {
	// https://www.twilio.com/docs/usage/security#validating-requests

	var buf strings.Builder
	buf.WriteString(baseUrl)

	if req.Method == "POST" {
		// Sort all POST fields alphabetically by key and concatenate
		// the parameter name and value to the end of the URL (with no
		// delimiter)
		err := req.ParseForm()
		if err != nil {
			return false, err
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
	expectedMAC := mac.Sum(nil)
	reqSignature, err := base64.StdEncoding.DecodeString(req.Header.Get("X-Twilio-Signature"))
	if err != nil {
		return false, err
	}

	return hmac.Equal(reqSignature, expectedMAC), nil
}

func TwilioValidatorMiddleware(authToken string, listenScheme string) func(http.Handler) http.Handler {
	f := func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, req *http.Request) {
			host := req.Host
			if !strings.Contains(host, ":") {
				host = fmt.Sprintf("%s:%d", req.Host, config.ListenPort)
			}

			baseUrl := fmt.Sprintf("%s://%s%s", listenScheme, host, req.URL.Path)

			ok, err := checkRequestSignature(baseUrl, req, authToken)
			if ok {
				h.ServeHTTP(w, req)
			} else {
				if err != nil {
					log.Print("Error while checking request signature: %s\n", err)
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
