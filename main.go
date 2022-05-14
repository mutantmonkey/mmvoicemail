package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
	"github.com/flosch/pongo2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
	ListenPort         uint     `json:"LISTEN_PORT"`
	CertFile           string   `json:"CERT_FILE"`
	KeyFile            string   `json:"KEY_FILE"`
}

type ConfigFlags struct {
	LocalOnly  bool
	ListenPort uint
	CertFile   string
	KeyFile    string
	ConfigFile string
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

func sendEmail(subject string, text string, remoteAddr string) error {
	var b bytes.Buffer

	var h mail.Header
	h.SetDate(time.Now())

	mailFrom, err := mail.ParseAddress(config.MailFrom)
	if err != nil {
		return err
	}

	h.SetAddressList("From", []*mail.Address{mailFrom})
	h.SetSubject(subject)
	h.Add("X-Mailer", "mmvoicemail")

	remoteIP, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		remoteIP = remoteAddr
	}
	h.Add("X-Originating-IP", remoteIP)

	var mailTo []*mail.Address
	for _, addr := range config.MailTo {
		parsed, err := mail.ParseAddress(addr)
		if err != nil {
			return err
		}
		mailTo = append(mailTo, parsed)
	}
	h.SetAddressList("To", mailTo)
	if err := h.GenerateMessageID(); err != nil {
		return err
	}

	mw, err := mail.CreateWriter(&b, h)
	if err != nil {
		return err
	}

	tw, err := mw.CreateInline()
	if err != nil {
		return err
	}
	var th mail.InlineHeader
	th.Set("Content-Type", "text/plain")
	w, err := tw.CreatePart(th)
	if err != nil {
		return err
	}
	io.WriteString(w, text)
	w.Close()
	tw.Close()
	mw.Close()

	server := config.SMTPServer
	if !strings.Contains(server, ":") {
		server = server + ":25"
	}

	hostname, _, err := net.SplitHostPort(server)
	if err != nil {
		return err
	}

	var auth smtp.Auth
	if config.SMTPUser != "" && config.SMTPPassword != "" {
		auth = smtp.PlainAuth("", config.SMTPUser, config.SMTPPassword, hostname)
	}

	err = smtp.SendMail(server, auth, config.MailFrom, config.MailTo, b.Bytes())
	if err != nil {
		return err
	}

	return nil
}

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

func main() {
	configFlags := &ConfigFlags{
		LocalOnly: false,
	}

	flag.BoolVar(&configFlags.LocalOnly, "local", false, "bind to localhost (no TLS)")
	flag.UintVar(&configFlags.ListenPort, "port", 0, "port to listen on")
	flag.StringVar(&configFlags.CertFile, "cert", "", "path to TLS certificate")
	flag.StringVar(&configFlags.KeyFile, "key", "", "path to TLS certificate key")
	flag.StringVar(&configFlags.ConfigFile, "config", "/etc/mmvoicemail/config.json", "path to config file")
	flag.Parse()

	if configFlags.ListenPort > 65535 {
		log.Fatal("invalid port: must be <65536")
	}

	var err error
	config, err = parseConfig(configFlags.ConfigFile)
	if err != nil {
		log.Fatal(err)
	}

	if configFlags.ListenPort != 0 {
		config.ListenPort = configFlags.ListenPort
	} else if config.ListenPort == 0 {
		config.ListenPort = 8080
	}

	if configFlags.CertFile != "" {
		config.CertFile = configFlags.CertFile
	}
	if configFlags.KeyFile != "" {
		config.KeyFile = configFlags.KeyFile
	}

	mux := chi.NewRouter()

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
		mux.Use(TwilioValidatorMiddleware(config.TwilioAuthToken))
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
			log.Print("error loading template: %s\n", err.Error())
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			return
		}
		body, err := tpl.Execute(context)
		if err != nil {
			log.Print("error executing template: %s\n", err.Error())
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
			log.Print("error loading template: %s\n", err.Error())
			http.Error(w, "500 internal server error", http.StatusInternalServerError)
			return
		}
		body, err := tpl.Execute(context)
		if err != nil {
			log.Print("error executing template: %s\n", err.Error())
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
	}

	var srv *http.Server
	if configFlags.LocalOnly {
		srv = &http.Server{
			Addr:         fmt.Sprintf("[::1]:%d", config.ListenPort),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
			TLSConfig:    tlsConfig,
			Handler:      mux,
		}
		log.Fatal(srv.ListenAndServe())
	} else {
		srv = &http.Server{
			Addr:         fmt.Sprintf(":%d", config.ListenPort),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  120 * time.Second,
			TLSConfig:    tlsConfig,
			Handler:      mux,
		}
		if config.CertFile != "" && config.KeyFile != "" {
			log.Fatal(srv.ListenAndServeTLS(config.CertFile, config.KeyFile))
		} else {
			log.Fatal(srv.ListenAndServe())
		}
	}
}
