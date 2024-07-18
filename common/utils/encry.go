package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func Sha256(text []byte) []byte {
	hash := sha256.New()
	hash.Write(text)
	return hash.Sum(nil)
}

type Aes struct {
	key     []byte
	vector  []byte
	block   cipher.Block
	mode    *CBCMode
	padding *PKCS7Padding
}

func NewAes(key []byte) *Aes {
	length := len(key)
	if length >= 16 {
		key = key[:16]
	} else {
		arr := make([]byte, 16)
		copy(arr, key)
		key = arr
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	return &Aes{
		key:     key,
		block:   block,
		vector:  key[:block.BlockSize()],
		mode:    &CBCMode{},
		padding: &PKCS7Padding{},
	}
}

// Encrypt aes
func (aes *Aes) Encrypt(origData string) (string, error) {
	data := []byte(origData)
	data = aes.padding.Padding(data, aes.block.BlockSize())
	ciphertext, err := aes.mode.Encrypt(aes.block, data, aes.key, aes.vector)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt URLEncoding
func (aes *Aes) DecryptRawURLEncoding(data string) (string, error) {
	ciphertext, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	origData, err := aes.mode.Decrypt(aes.block, ciphertext, aes.key, aes.vector)
	if err != nil {
		return "", err
	}
	return string(aes.padding.UnPadding(origData)), nil
}

// Decrypt
func (aes *Aes) Decrypt(data string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	origData, err := aes.mode.Decrypt(aes.block, ciphertext, aes.key, aes.vector)
	if err != nil {
		return "", err
	}
	return string(aes.padding.UnPadding(origData)), nil
}

type PKCS7Padding struct{}

func (PKCS7Padding) Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func (PKCS7Padding) UnPadding(origData []byte) []byte {
	length := len(origData)
	unpadding := int(origData[length-1])
	return origData[:(length - unpadding)]
}

type CBCMode struct{}

func (CBCMode) Encrypt(block cipher.Block, data, key, iv []byte) ([]byte, error) {
	bs := block.BlockSize()
	if len(data)%bs != 0 {
		return nil, errors.New("Need a multiple of the blocksize")
	}
	ciphertext := make([]byte, len(data))
	blockMode := cipher.NewCBCEncrypter(block, iv)
	blockMode.CryptBlocks(ciphertext, data)
	return ciphertext, nil
}

func (CBCMode) Decrypt(block cipher.Block, data, key, iv []byte) ([]byte, error) {
	bs := block.BlockSize()
	if len(data)%bs != 0 {
		return nil, errors.New("input not full blocks")
	}
	plaintext := make([]byte, len(data))
	blockMode := cipher.NewCBCDecrypter(block, iv)
	blockMode.CryptBlocks(plaintext, data)
	return plaintext, nil
}

func MD5(plainText string) string {
	h := md5.New()
	_, err := h.Write([]byte(plainText))
	if err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

type Cover struct {
	Timestamp string
	Body      string
	Sign      string
	AppKey    string
}

func NewCover(appKey string, body interface{}) (*Cover, error) {
	jsonStr, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &Cover{
		Timestamp: fmt.Sprintf("%d", time.Now().Unix()),
		AppKey:    appKey,
		Body:      string(jsonStr),
	}, nil
}

func (c *Cover) ValidateSign(signParam string) error {
	err := c.HmacSHA256Sign()
	if err != nil {
		return fmt.Errorf("Sign err")
	}
	if c.Sign != signParam {
		return fmt.Errorf("Sign err: correctSign：%s appKey：%s body：%s", c.Sign, c.AppKey, c.Body)
	}
	return nil
}

// sign
func (c *Cover) HmacSHA256Sign() error {
	mac := hmac.New(sha256.New, []byte(c.AppKey))
	_, err := mac.Write([]byte(c.Body + c.Timestamp))
	if err != nil {
		return err
	}
	c.Sign = hex.EncodeToString(mac.Sum(nil))
	return nil
}
