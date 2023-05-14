package main

import (
	"flag"
	"fmt"
	"log"
	"net/smtp"
)

var addrFlag = flag.String("addr", "localhost:8025", "the address to send the email to.")
var fromFlag = flag.String("from", "sender@example.com", "the sender's email address.")
var toFlag = flag.String("to", "recipient@example.com", "the recipient's email address.")
var subjectFlag = flag.String("subject", "email subject here", "the email subject.")
var bodyFlag = flag.String("body", "email body here.\n", "the email's body.")

func run() error {
	flag.Parse()
	message := fmt.Sprintf("from: <%s>\r\nto: <%s>\r\nsubject: %s\r\n\r\n%s", *fromFlag, *toFlag, *subjectFlag, *bodyFlag)
	return smtp.SendMail("localhost:8025", nil, *fromFlag, []string{*toFlag}, []byte(message))
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
