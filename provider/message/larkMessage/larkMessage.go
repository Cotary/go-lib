package larkMessage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Cotary/go-lib/common/utils"
	http2 "github.com/Cotary/go-lib/net/http"
	"github.com/pkg/errors"
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

func (t LarkRobot) SendMessage(ctx context.Context, language string, title string, zMap *utils.OrderedMap[string, string], atList []string) (*http2.Response, error) {
	m := genMsg(language, title, zMap, atList)

	//send
	if err := m.genSign(t.Secret); err != nil {
		return nil, errors.Wrap(err, "sign err")
	}

	str, err := json.Marshal(m)
	if err != nil {
		return nil, errors.Wrap(err, "json.Marshal err")
	}

	url := fmt.Sprintf("https://open.larksuite.com%s", t.RobotPath)
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	res := http2.FastHTTP().Execute(ctx, http.MethodPost, url, nil, str, headers)
	return res.Response, res.Error
}

type content struct {
	Title   string      `json:"title,omitempty"`
	Content [][]content `json:"content,omitempty"`
	UserID  string      `json:"user_id,omitempty"`
	Text    string      `json:"text,omitempty"`
	Tag     string      `json:"tag,omitempty"`
}

// genMsg 根据飞书文档生成 post 消息格式
// 参考: https://open.larksuite.com/document/client-docs/bot-v3/use-custom-bots-in-a-group#f62e72d5
// 消息格式: 每行是一个数组，包含多个 content 元素，键值对在同一行左右排列
func genMsg(language string, title string, zMap *utils.OrderedMap[string, string], atList []string) *larkMessage {
	m := new(larkMessage)
	m.MsgType = "post"
	m.Content = make(map[string]map[string]*content)
	m.Content["post"] = make(map[string]*content)

	messageContent := make([][]content, 0)

	// 处理键值对：每对键值在同一行，左右排列
	if zMap != nil {
		zMap.Each(func(p utils.Pair[string, string]) bool {
			key := p.Key
			value := p.Value
			if key == "" {
				key = " "
			}
			if value == "" {
				value = " "
			}
			messageContent = append(messageContent, []content{
				{Tag: "text", Text: key + ": "},
				{Tag: "text", Text: value},
			})
			return true
		})
	}

	// 处理 @ 用户列表：所有 @ 用户在同一行
	if len(atList) > 0 {
		atRow := make([]content, 0, len(atList))
		for _, userID := range atList {
			if userID != "" {
				atRow = append(atRow, content{
					Tag:    "at",
					UserID: userID,
				})
			}
		}
		if len(atRow) > 0 {
			messageContent = append(messageContent, atRow)
		}
	}

	m.Content["post"][language] = &content{
		Title:   title,
		Content: messageContent,
	}
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
