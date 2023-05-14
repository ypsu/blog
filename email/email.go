package email

import (
	"log"
	"net/mail"
)

func EmailHandler(from string, rcpt []string, msg *mail.Message) {
	log.Printf("email from %q to %q subject %q", from, rcpt, msg.Header.Get("subject"))
}
