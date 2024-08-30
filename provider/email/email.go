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
	Smtp     string `yaml:"smtp"`
	Port     int    `yaml:"port"`
}

type Email struct {
	config *Config
}

func NewEmail(config *Config) *Email {
	return &Email{config: config}
}

func (e *Email) QQEmail() *Email {
	e.config.Smtp = "smtp.qq.com"
	e.config.Port = 25
	return e
}

func (e *Email) Gmail() *Email {
	e.config.Smtp = "smtp.gmail.com"
	e.config.Port = 587
	return e
}

func (e *Email) Send(email *jemail.Email) error {
	if email.From == "" {
		email.From = e.config.UserName
	}
	return email.Send(
		fmt.Sprintf("%s:%d", e.config.Smtp, e.config.Port),
		smtp.PlainAuth(e.config.Identity, e.config.UserName, e.config.Password, e.config.Smtp),
	)
}
