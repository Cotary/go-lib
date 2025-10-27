package email

import (
	"crypto/tls"
	"fmt"
	jemail "github.com/jordan-wright/email"
	"github.com/pkg/errors"
	"net/smtp"
)

// 定义 TLS 模式常量
const (
	// TlsModelNone 不使用 TLS，通常用于端口 25
	TlsModelNone = 0
	// TlsModelTLS 使用隐式 TLS/SSL，通常用于端口 465
	TlsModelTLS = 1
	// TlsModelStartTLS 使用 STARTTLS，通常用于端口 587
	TlsModelStartTLS = 2
)

type Config struct {
	Identity           string `mapstructure:"identity" yaml:"identity"`
	UserName           string `mapstructure:"userName" yaml:"userName"`
	Password           string `mapstructure:"password" yaml:"password"`
	SmtpHost           string `mapstructure:"smtp" yaml:"smtp"`
	Port               int    `mapstructure:"port" yaml:"port"`
	TlsModel           int    `mapstructure:"tlsModel" yaml:"tlsModel"` // 0不使用，1 tls（465 端口）, 2 starttls（587 端口）
	InsecureSkipVerify bool   `mapstructure:"insecureSkipVerify" yaml:"insecureSkipVerify"`
	CertFile           string `mapstructure:"certFile" yaml:"certFile"`
	KeyFile            string `mapstructure:"keyFile" yaml:"keyFile"`
}

type Email struct {
	config Config
}

func NewEmail(config Config) *Email {
	return &Email{config: config}
}

func (e *Email) QQEmail() *Email {
	e.config.SmtpHost = "smtp.qq.com"
	e.config.TlsModel = TlsModelTLS // 使用新常量
	e.config.Port = 465
	return e
}

func (e *Email) Gmail() *Email {
	e.config.SmtpHost = "smtp.gmail.com"
	e.config.TlsModel = TlsModelStartTLS // 使用新常量
	e.config.Port = 587
	return e
}

func (e *Email) TlsNone() *Email {
	e.config.TlsModel = TlsModelNone // 使用新常量
	e.config.Port = 25
	return e
}

func (e *Email) Tls() *Email {
	e.config.TlsModel = TlsModelTLS // 使用新常量
	e.config.Port = 465
	return e
}

func (e *Email) StartTls() *Email {
	e.config.TlsModel = TlsModelStartTLS // 使用新常量
	e.config.Port = 587
	return e
}

func (e *Email) Send(email *jemail.Email) error {
	// 1. 预检查
	if e.config.SmtpHost == "" || e.config.Port == 0 {
		return errors.New("SMTP host or port is empty")
	}
	if email.From == "" {
		email.From = e.config.UserName
	}

	// 2. 身份验证和地址
	auth := smtp.PlainAuth(e.config.Identity, e.config.UserName, e.config.Password, e.config.SmtpHost)
	address := fmt.Sprintf("%s:%d", e.config.SmtpHost, e.config.Port)

	// 3. TLS 配置（仅在需要时创建）
	if e.config.TlsModel == TlsModelTLS || e.config.TlsModel == TlsModelStartTLS {
		tlsConfig, err := e.getTLSConfig()
		if err != nil {
			return errors.Wrap(err, "failed to load TLS certificate")
		}

		// 4. 根据 TLS 模式发送邮件
		switch e.config.TlsModel {
		case TlsModelTLS:
			return email.SendWithTLS(address, auth, tlsConfig)
		case TlsModelStartTLS:
			return email.SendWithStartTLS(address, auth, tlsConfig)
		}
	}

	// 5. 不使用 TLS (TlsModelNone) 时发送邮件
	return email.Send(address, auth)
}

// getTLSConfig 提取了创建和加载 TLS 配置的逻辑
func (e *Email) getTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: e.config.InsecureSkipVerify,
		ServerName:         e.config.SmtpHost,
	}

	if e.config.CertFile != "" && e.config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(e.config.CertFile, e.config.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
}
