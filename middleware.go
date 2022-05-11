package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/twilio/twilio-go/client"
)

func TwilioValidatorMiddleware(authToken string) func(http.Handler) http.Handler {
	rv := client.NewRequestValidator(config.TwilioAuthToken)
	f := func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, req *http.Request) {
			body, err := ioutil.ReadAll(req.Body)
			if err != nil {
				http.Error(w, "403 forbidden", http.StatusForbidden)
			}
			if !rv.ValidateBody(req.URL.String(), body, req.Header.Get("X-Twilio-Signature")) {
				http.Error(w, "403 forbidden", http.StatusForbidden)
			}
			h.ServeHTTP(w, req)
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
