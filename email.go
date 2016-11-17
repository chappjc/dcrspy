package main

import (
	"errors"
	"fmt"
	"net/smtp"
	"strconv"
)

type emailConfig struct {
	emailAddr                      string
	smtpUser, smtpPass, smtpServer string
	smtpPort                       int
}

// sendEmailWatchRecv Sends an email using the input emailConfig and message
// string.
func sendEmailWatchRecv(message string, ecfg *emailConfig) error {
	// Check for nil pointer emailConfig
	if ecfg == nil {
		return errors.New("emailConfig must not be a nil pointer")
	}

	auth := smtp.PlainAuth(
		"",
		ecfg.smtpUser,
		ecfg.smtpPass,
		ecfg.smtpServer,
	)

	// The SMTP server address includes the port
	addr := ecfg.smtpServer + ":" + strconv.Itoa(ecfg.smtpPort)
	//log.Debug(addr)

	// Make a header using a map for clarity
	header := make(map[string]string)
	header["From"] = ecfg.smtpUser
	header["To"] = ecfg.emailAddr
	// TODO: make subject line adjustable or include an amount
	header["Subject"] = "dcrspy notification"
	//header["MIME-Version"] = "1.0"
	//header["Content-Type"] = "text/plain; charset=\"utf-8\""
	//header["Content-Transfer-Encoding"] = "base64"

	// Build the full message with the header + input message string
	messageFull := ""
	for k, v := range header {
		messageFull += fmt.Sprintf("%s: %s\r\n", k, v)
	}

	messageFull += "\r\n" + message

	// Send email
	err := smtp.SendMail(
		addr,
		auth,
		ecfg.smtpUser,            // sender is receiver
		[]string{ecfg.emailAddr}, // recipients
		[]byte(messageFull),
	)

	if err != nil {
		log.Errorf("Failed to send email: %v", err)
		return err
	}

	log.Tracef("Send email to address %v\n", ecfg.emailAddr)
	return nil
}
