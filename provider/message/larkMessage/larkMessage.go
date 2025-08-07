package larkMessage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/Cotary/go-lib/common/utils"
	http2 "github.com/Cotary/go-lib/net/http"
	"github.com/coocood/freecache"
	"github.com/pkg/errors"
	"net/http"
	"time"
)

type LarkRobot struct {
	RobotPath string
	Secret    string
}

func NewLarkRobot(robotPath, secret string) *LarkRobot {
	return &LarkRobot{
		RobotPath: robotPath,
		Secret:    secret,
	}
}

type larkMessage struct {
	Timestamp string                         `json:"timestamp"`
	MsgType   string                         `json:"msg_type"`
	Content   map[string]map[string]*content `json:"content"`
	Sign      string                         `json:"sign"`
}

func (t LarkRobot) SendMessage(language string, title string, message []string, atList []string, intercept bool) (*http2.Response, error) {
	m := genMsg(language, title, message, atList)

	//send
	if err := m.genSign(t.Secret); err != nil {
		return nil, errors.Wrap(err, "sign err")
	}

	str, err := json.Marshal(m)
	if err != nil {
		return nil, errors.Wrap(err, "json.Marshal err")
	}
	if !intercept {
		cacheKey, _ := json.Marshal(m.Content)
		cacheVal, _ := cache.GetOrSet([]byte(utils.MD5(string(cacheKey))), []uint8{1}, 60)
		if len(cacheVal) > 0 {
			return nil, nil
		}
	}

	url := fmt.Sprintf("https://open.larksuite.com%s", t.RobotPath)
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	res := http2.NewRequestBuilder(http2.DefaultFastHTTPClient).Execute(context.Background(), http.MethodPost, url, nil, str, headers)
	return res.Response, res.Error
}

type content struct {
	Title   string      `json:"title,omitempty"`
	Content [][]content `json:"content,omitempty"`
	UserID  string      `json:"user_id,omitempty"`
	Text    string      `json:"text,omitempty"`
	Tag     string      `json:"tag,omitempty"`
}

var cache = freecache.NewCache(10 * 1024 * 1024)

// https://open.larksuite.com/document/client-docs/bot-v3/use-custom-bots-in-a-group#f62e72d5
func genMsg(language string, title string, message []string, atList []string) *larkMessage {
	//gen msg
	m := new(larkMessage)
	m.MsgType = "post"
	m.Content = make(map[string]map[string]*content)
	m.Content["post"] = make(map[string]*content)

	messageContent := make([][]content, 0)
	messageRow := make([]content, 0)
	for _, v := range message {
		if v == "" {
			v = " "
		}
		messageRow = append(messageRow, content{
			Tag:  "text",
			Text: v,
		})
		if len(messageRow) == 2 {
			messageContent = append(messageContent, messageRow)
			messageRow = make([]content, 0)
		}
	}
	if len(atList) > 0 {
		messageRow = make([]content, 0)
		for _, userID := range atList {
			messageRow = append(messageRow, content{
				Tag:    "at",
				UserID: userID,
			})
		}
		messageContent = append(messageContent, messageRow)
	}
	m.Content["post"][language] = &content{Title: title, Content: make([][]content, 0)}
	m.Content["post"][language].Content = messageContent
	return m
}

func (m *larkMessage) genSign(secret string) error {
	timestamp := time.Now().Unix()
	m.Timestamp = fmt.Sprintf("%d", timestamp)
	stringToSign := m.Timestamp + "\n" + secret
	var data []byte
	h := hmac.New(sha256.New, []byte(stringToSign))
	_, err := h.Write(data)
	if err != nil {
		return err
	}
	m.Sign = base64.StdEncoding.EncodeToString(h.Sum(nil))
	return nil
}
