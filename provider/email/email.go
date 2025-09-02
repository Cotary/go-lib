package email

import (
	"crypto/tls"
	"fmt"
	jemail "github.com/jordan-wright/email"
	"net/smtp"
)

type Config struct {
	Identity           string `mapstructure:"identity"`
	UserName           string `mapstructure:"userName"`
	Password           string `mapstructure:"password"`
	Smtp               string `mapstructure:"smtp"`
	Port               int    `mapstructure:"port"`
	TlsModel           int    `mapstructure:"tlsModel"` // 0不使用，1 tls, 2 starttls
	InsecureSkipVerify bool   `mapstructure:"insecureSkipVerify"`
	CertFile           string `mapstructure:"certFile"`
	KeyFile            string `mapstructure:"keyFile"`
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

	var tlsConfig *tls.Config
	if e.config.TlsModel == 1 || e.config.TlsModel == 2 {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: e.config.InsecureSkipVerify,
			ServerName:         e.config.Smtp,
		}

		if e.config.CertFile != "" && e.config.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(e.config.CertFile, e.config.KeyFile)
			if err != nil {
				return err
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
	}

	auth := smtp.PlainAuth(e.config.Identity, e.config.UserName, e.config.Password, e.config.Smtp)
	address := fmt.Sprintf("%s:%d", e.config.Smtp, e.config.Port)

	switch e.config.TlsModel {
	case 1:
		return email.SendWithTLS(address, auth, tlsConfig)
	case 2:
		return email.SendWithStartTLS(address, auth, tlsConfig)
	default:
		return email.Send(address, auth)
	}
}
