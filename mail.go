package main

import (
	"bytes"
	"io"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
)

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
