package email

import (
	"fmt"
	jemail "github.com/jordan-wright/email"
	"net/smtp"
)

type Config struct {
	Identity string `yaml:"identity"`
	UserName string `yaml:"userName"`
	Password string `yaml:"password"`
	smtp     string `yaml:"smtp"`
	port     int    `yaml:"port"`
}

type Email struct {
	config *Config
}

func NewEmail(config *Config) *Email {
	return &Email{config: config}
}

func (e *Email) QQEmail() *Email {
	e.config.smtp = "smtp.qq.com"
	e.config.port = 25
	return e
}

func (e *Email) Gmail() *Email {
	e.config.smtp = "smtp.gmail.com"
	e.config.port = 587
	return e
}

func (e *Email) Send(email *jemail.Email) error {
	if email.From == "" {
		email.From = e.config.UserName
	}
	return email.Send(
		fmt.Sprintf("%s:%d", e.config.smtp, e.config.port),
		smtp.PlainAuth(e.config.Identity, e.config.UserName, e.config.Password, e.config.smtp),
	)
}
