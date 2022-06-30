package main

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/flosch/pongo2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/crypto/acme"
	"src.agwa.name/go-listener/cert"
)

type Config struct {
	MailFrom           string   `json:"MAIL_FROM"`
	MailTo             []string `json:"MAIL_TO"`
	SMTPServer         string   `json:"SMTP_SERVER"`
	SMTPServerTLS      bool     `json:"SMTP_TLS"`
	SMTPUser           string   `json:"SMTP_USER"`
	SMTPPassword       string   `json:"SMTP_PASSWORD"`
	TwilioAuthToken    string   `json:"TWILIO_AUTH_TOKEN"`
	ProxyFix           bool     `json:"PROXY_FIX"`
	ProxyFixNumProxies int      `json:"PROXY_FIX_NUM_PROXIES"`
	ListenPort         string   `json:"LISTEN_PORT"`
	CertFile           string   `json:"CERT_FILE"`
	AutocertHostnames  []string `json:"AUTOCERT_HOSTNAMES"`
}

type ConfigFlags struct {
	LocalOnly         bool
	ListenPort        string
	CertFile          string
	ConfigFile        string
	AutocertHostnames string
}

type RecordFinishedResponse struct {
	CallSid      string
	From         string
	FromCity     string
	FromState    string
	FromCountry  string
	To           string
	ToCity       string
	ToState      string
	ToCountry    string
	RecordingUrl string
}

type SMSResponse struct {
	From string
	To   string
	Body string
}

var config *Config
var configFlags *ConfigFlags

func stripCRLF(input string) (output string) {
	output = strings.ReplaceAll(input, "\r", "")
	output = strings.ReplaceAll(output, "\n", "")
	return
}

func parseConfig(path string) (c *Config, err error) {
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()

	body, err := ioutil.ReadAll(f)
	if err != nil {
		return c, err
	}

	err = json.Unmarshal(body, &c)
	if err != nil {
		return
	}

	return
}

func openListener(config *Config, mux *chi.Mux, localOnly bool) error {
	var getCertificate cert.GetCertificateFunc
	var nextProtos = []string{"h2", "http/1.1"}

	if localOnly {
		getCertificate = nil
	} else if config.CertFile != "" {
		getCertificate = cert.GetCertificateFromFile(config.CertFile)
	} else if len(config.AutocertHostnames) > 0 {
		getCertificate = cert.GetCertificateAutomatically(config.AutocertHostnames)
		nextProtos = append(nextProtos, acme.ALPNProto)
	} else {
		return errors.New("certificate not specified for TLS listener")
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
		// Causes servers to use Go's default ciphersuite preferences,
		// which are tuned to avoid attacks. Does nothing on clients.
		PreferServerCipherSuites: true,
		// Only use curves which have assembly implementations
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519,
		},
		GetCertificate: getCertificate,
		NextProtos:     nextProtos,
	}

	var srv *http.Server
	srv = &http.Server{
		Addr:         config.ListenPort,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig:    tlsConfig,
		Handler:      mux,
	}
	if !localOnly && (config.CertFile != "" || len(config.AutocertHostnames) > 0) {
		return srv.ListenAndServeTLS("", "")
	} else {
		return srv.ListenAndServe()
	}
}

func main() {
	configFlags := &ConfigFlags{
		LocalOnly: false,
	}

	flag.BoolVar(&configFlags.LocalOnly, "local", false, "bind to localhost (no TLS)")
	flag.StringVar(&configFlags.ListenPort, "port", "", "port to listen on (or host:port)")
	flag.StringVar(&configFlags.CertFile, "cert", "", "path to concatenated TLS certificate and private key")
	flag.StringVar(&configFlags.ConfigFile, "config", "/etc/mmvoicemail/config.json", "path to config file")
	flag.StringVar(&configFlags.AutocertHostnames, "hostname", "", "comma-separated list of hostnames to use when automatically obtaining certificate")
	flag.Parse()

	var err error
	config, err = parseConfig(configFlags.ConfigFile)
	if err != nil {
		log.Fatal(err)
	}

	if len(configFlags.ListenPort) > 0 {
		config.ListenPort = configFlags.ListenPort
	}

	if len(config.ListenPort) == 0 {
		if configFlags.LocalOnly {
			config.ListenPort = "[::1]:8080"
		} else {
			config.ListenPort = ":443"
		}
	} else if !strings.Contains(config.ListenPort, ":") {
		config.ListenPort = fmt.Sprintf(":%s", config.ListenPort)
	}

	if configFlags.CertFile != "" {
		config.CertFile = configFlags.CertFile
	}
	if configFlags.AutocertHostnames != "" {
		config.AutocertHostnames = strings.Split(configFlags.AutocertHostnames, ",")
	}

	mux := chi.NewRouter()
	mux.Use(SecurityHeaderMiddleware)

	if config.ProxyFix {
		if config.ProxyFixNumProxies < 1 {
			config.ProxyFixNumProxies = 1
		}
		mux.Use(ProxyFixMiddleware(config.ProxyFixNumProxies))
	}

	mux.Use(middleware.Logger)
	mux.Use(middleware.Heartbeat("/"))

	// If Twilio auth token is defined, add middleware that requires a
	// valid Twilio signature.
	if config.TwilioAuthToken != "" {
		baseUrl := &url.URL{
			Scheme: "https",
			Host:   config.ListenPort,
		}

		// the "http" scheme may only be used for local testing
		// otherwise, assume we're being proxied by an HTTPS frontend
		if configFlags.LocalOnly {
			baseUrl.Scheme = "http"
		}

		mux.Use(TwilioValidatorMiddleware(config.TwilioAuthToken, baseUrl))
	} else {
		log.Print("Warning: request validation is disabled because TWILIO_AUTH_TOKEN was not provided in the config.")
	}

	mux.Post("/record/start.xml", func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, "static/record.xml")
	})
	mux.Post("/record/finished.xml", func(w http.ResponseWriter, req *http.Request) {
		context := pongo2.Context{
			"CallSid":      req.PostFormValue("CallSid"),
			"From":         req.PostFormValue("From"),
			"FromCity":     req.PostFormValue("FromCity"),
			"FromCountry":  req.PostFormValue("FromCountry"),
			"To":           req.PostFormValue("To"),
			"ToCity":       req.PostFormValue("ToCity"),
			"ToState":      req.PostFormValue("ToState"),
			"ToCountry":    req.PostFormValue("ToCountry"),
			"RecordingUrl": req.PostFormValue("RecordingUrl"),
		}

		tpl, err := pongo2.FromFile("templates/voicemail_email.txt")
		if err != nil {
			log.Printf("error loading template: %s\n", err.Error())
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			return
		}
		body, err := tpl.Execute(context)
		if err != nil {
			log.Printf("error executing template: %s\n", err.Error())
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			return
		}

		err = sendEmail(
			fmt.Sprintf("Voicemail from %s", stripCRLF(req.PostFormValue("From"))),
			body, req.RemoteAddr)
		if err != nil {
			log.Print(fmt.Sprintf("error sending email: %s\n", err))
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			return
		}

		http.ServeFile(w, req, "static/goodbye.xml")
	})
	mux.Post("/sms", func(w http.ResponseWriter, req *http.Request) {
		context := pongo2.Context{
			"From": req.PostFormValue("From"),
			"To":   req.PostFormValue("To"),
			"Body": req.PostFormValue("Body"),
		}

		tpl, err := pongo2.FromFile("templates/sms_email.txt")
		if err != nil {
			log.Printf("error loading template: %s\n", err.Error())
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			return
		}
		body, err := tpl.Execute(context)
		if err != nil {
			log.Printf("error executing template: %s\n", err.Error())
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			return
		}

		err = sendEmail(
			fmt.Sprintf("SMS from %s", stripCRLF(req.PostFormValue("From"))),
			body, req.RemoteAddr)
		if err != nil {
			log.Print(fmt.Sprintf("error sending email: %s\n", err))
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			return
		}
	})

	err = openListener(config, mux, configFlags.LocalOnly)
	if err != nil {
		log.Fatalf("error opening listener: %s\n", err)
		return
	}
}
