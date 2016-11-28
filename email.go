package main

import (
	"fmt"
	"math"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type EmailConfig struct {
	emailAddr                      string
	smtpUser, smtpPass, smtpServer string
	smtpPort                       int
}

// EmailMsgChan is used with EmailQueue to automatically batch messages in to
// single emails.
var EmailMsgChan chan string

func init() {
	EmailMsgChan = make(chan string, 200)
}

// SendEmailWatchRecv Sends an email using the input emailConfig and message
// string.
func SendEmailWatchRecv(message, subject string, ecfg *EmailConfig) error {
	// Check for nil pointer emailConfig
	if ecfg == nil {
		return fmt.Errorf("emailConfig must not be a nil pointer")
	}

	auth := smtp.PlainAuth(
		"",
		ecfg.smtpUser,
		ecfg.smtpPass,
		ecfg.smtpServer,
	)

	// The SMTP server address includes the port
	addr := ecfg.smtpServer + ":" + strconv.Itoa(ecfg.smtpPort)

	// Make a header using a map for clarity
	header := make(map[string]string)
	header["From"] = ecfg.smtpUser
	header["To"] = ecfg.emailAddr
	header["Subject"] = subject
	//header["MIME-Version"] = "1.0"
	header["Content-Type"] = `text/plain; charset="utf-8"`
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
		return fmt.Errorf("Failed to send email: %v", err)
	}

	return nil
}

// sendEmailWatchRecv is launched as a goroutine by EmailQueue
func sendEmailWatchRecv(message, subject string, ecfg *EmailConfig) {
	err := SendEmailWatchRecv(message, subject, ecfg)
	if err != nil {
		log.Warn(err)
		return
	}
	log.Debugf("Sent email to %v", ecfg.emailAddr)
}

// EmailQueue batches messages into single emails, using a progressively shorter
// delay before sending an email as the number of queued messages increases.
// Messages are received on the package-level channel mpEmailMsgChan. emailQueue
// should be run as a goroutine.
func EmailQueue(emailConf *EmailConfig, subject string,
	wg *sync.WaitGroup, quit <-chan struct{}) {
	defer wg.Done()

	msgIntro := "Watched addresses were observed in the following transactions:\n\n"

	var msgStrings []string
	lastMsgTime := time.Now()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeToWait := func(numMessages int) time.Duration {
		if numMessages == 0 {
			return math.MaxInt64
		}
		return 10 * time.Second / time.Duration(numMessages)
	}

	for {
		//watchquit:
		select {
		case <-quit:
			log.Debugf("Quitting emailQueue.")
			return
		case msg, ok := <-EmailMsgChan:
			if !ok {
				log.Info("emailQueue channel closed")
				return
			}
			msgStrings = append(msgStrings, msg)
			lastMsgTime = time.Now()
		case <-ticker.C:
			if time.Since(lastMsgTime) > timeToWait(len(msgStrings)) {
				go sendEmailWatchRecv(msgIntro+strings.Join(msgStrings, "\n\n"),
					subject, emailConf)
				msgStrings = nil
			}
		}
	}
}
