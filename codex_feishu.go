package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ================= 配置区域 =================
// 运行前请在环境变量中设置以下配置:
//   FEISHU_WEBHOOK_URL - 飞书群机器人提供的完整 Webhook URL (必填)
//   FEISHU_SECRET      - 如果开启签名校验, 填写机器人安全设置中的 Secret (选填)
// ===========================================

// CodexNotification 定义 Codex 传入的 JSON 结构
type CodexNotification struct {
	Type                 string   `json:"type"`
	ThreadID             string   `json:"thread-id"`
	TurnID               string   `json:"turn-id"`
	Cwd                  string   `json:"cwd"`
	InputMessages        []string `json:"input-messages"`
	LastAssistantMessage string   `json:"last-assistant-message"`
}

// ================= 飞书卡片消息结构定义 =================

type FeishuCardMsg struct {
	Timestamp string     `json:"timestamp,omitempty"` // 认证字段: 秒级时间戳
	Sign      string     `json:"sign,omitempty"`      // 认证字段: 签名
	MsgType   string     `json:"msg_type"`
	Card      FeishuCard `json:"card"`
}

type FeishuCard struct {
	Config   FeishuCardConfig `json:"config,omitempty"`
	Header   FeishuHeader     `json:"header"`
	Elements []interface{}    `json:"elements"`
}

type FeishuCardConfig struct {
	WideScreenMode bool `json:"wide_screen_mode"`
}

type FeishuHeader struct {
	Title    FeishuText `json:"title"`
	Template string     `json:"template"`
}

type FeishuText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type FeishuDiv struct {
	Tag    string        `json:"tag"`
	Fields []FeishuField `json:"fields,omitempty"`
	Text   *FeishuText   `json:"text,omitempty"`
}

type FeishuField struct {
	IsShort bool       `json:"is_short"`
	Text    FeishuText `json:"text"`
}

type FeishuNote struct {
	Tag      string       `json:"tag"`
	Elements []FeishuText `json:"elements"`
}

type FeishuHr struct {
	Tag string `json:"tag"`
}

// ======================================================

type FeishuConfig struct {
	WebhookURL string
	Secret     string
}

type FeishuResponse struct {
	Code          int    `json:"code"`
	Msg           string `json:"msg"`
	StatusCode    int    `json:"StatusCode"`
	StatusMessage string `json:"StatusMessage"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: codex-notify <NOTIFICATION_JSON>")
		os.Exit(1)
	}

	jsonStr := os.Args[1]

	cfg, err := loadConfig()
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
		os.Exit(1)
	}

	var notification CodexNotification
	err = json.Unmarshal([]byte(jsonStr), &notification)
	if err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	if notification.Type == "agent-turn-complete" {
		if err := sendFeishuCard(notification, cfg); err != nil {
			fmt.Printf("Failed to send notification: %v\n", err)
			os.Exit(1)
		}
	}
}

func loadConfig() (FeishuConfig, error) {
	webhook := strings.TrimSpace(os.Getenv("FEISHU_WEBHOOK_URL"))
	if webhook == "" {
		return FeishuConfig{}, errors.New("FEISHU_WEBHOOK_URL is not set")
	}
	secret := strings.TrimSpace(os.Getenv("FEISHU_SECRET"))
	return FeishuConfig{
		WebhookURL: webhook,
		Secret:     secret,
	}, nil
}

// GenSign 生成飞书自定义机器人所需的签名
// 算法: base64(hmac_sha256(key=timestamp+"\n"+secret, msg=""))
func GenSign(secret string, timestamp int64) (string, error) {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	var data []byte
	h := hmac.New(sha256.New, []byte(stringToSign))
	_, err := h.Write(data)
	if err != nil {
		return "", err
	}
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return signature, nil
}

func sendFeishuCard(n CodexNotification, cfg FeishuConfig) error {
	// 1. 准备基础数据
	userIntent := "Unknown Task"
	if len(n.InputMessages) > 0 {
		userIntent = n.InputMessages[0]
	}

	displayTitle := truncateRunes(userIntent, 30)

	// 2. 计算签名 (如果配置了 Secret)
	var timestampStr, sign string
	if cfg.Secret != "" {
		ts := time.Now().Unix()
		timestampStr = strconv.FormatInt(ts, 10)
		var err error
		sign, err = GenSign(cfg.Secret, ts)
		if err != nil {
			return fmt.Errorf("sign generation failed: %v", err)
		}
	}

	// 3. 构建卡片元素
	var elements []interface{}

	// 元素: 输入指令
	inputContent := strings.Join(n.InputMessages, "\n")
	elements = append(elements, FeishuDiv{
		Tag: "div",
		Text: &FeishuText{
			Tag:     "lark_md",
			Content: fmt.Sprintf("**📝 输入指令:**\n%s", inputContent),
		},
	})

	elements = append(elements, FeishuHr{Tag: "hr"})

	// 元素: 执行结果
	resultContent := strings.TrimSpace(n.LastAssistantMessage)
	if resultContent == "" {
		resultContent = "（无执行结果描述）"
	}
	resultContent = truncateRunes(resultContent, 500)
	elements = append(elements, FeishuDiv{
		Tag: "div",
		Text: &FeishuText{
			Tag:     "lark_md",
			Content: fmt.Sprintf("**✅ 执行结果:**\n%s", resultContent),
		},
	})

	elements = append(elements, FeishuHr{Tag: "hr"})

	// 元素: 路径与ID
	elements = append(elements, FeishuDiv{
		Tag: "div",
		Fields: []FeishuField{
			{
				IsShort: true,
				Text: FeishuText{
					Tag:     "lark_md",
					Content: fmt.Sprintf("**📂 工作路径:**\n`%s`", n.Cwd),
				},
			},
			{
				IsShort: true,
				Text: FeishuText{
					Tag:     "lark_md",
					Content: fmt.Sprintf("**🆔 Thread ID:**\n`%s`", n.ThreadID),
				},
			},
		},
	})

	// 元素: 底部备注
	elements = append(elements, FeishuNote{
		Tag: "note",
		Elements: []FeishuText{
			{
				Tag:     "plain_text",
				Content: fmt.Sprintf("Generated by Codex at %s", time.Now().Format("15:04:05")),
			},
		},
	})

	// 4. 组装完整消息体
	cardMsg := FeishuCardMsg{
		Timestamp: timestampStr, // 只有当配置了 secret 时，这才有意义，但传了也无妨
		Sign:      sign,         // 签名
		MsgType:   "interactive",
		Card: FeishuCard{
			Config: FeishuCardConfig{WideScreenMode: true},
			Header: FeishuHeader{
				Template: "indigo",
				Title: FeishuText{
					Tag:     "plain_text",
					Content: fmt.Sprintf("🤖 Codex 任务完成: %s", displayTitle),
				},
			},
			Elements: elements,
		},
	}

	payloadBytes, err := json.Marshal(cardMsg)
	if err != nil {
		return err
	}

	// 5. 发送请求
	req, err := http.NewRequest("POST", cfg.WebhookURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status: %d, resp: %s", resp.StatusCode, string(bodyBytes))
	}

	var feishuResp FeishuResponse
	if err := json.Unmarshal(bodyBytes, &feishuResp); err != nil {
		return fmt.Errorf("decode feishu response: %w (payload: %s)", err, string(bodyBytes))
	}
	if feishuResp.Code != 0 || feishuResp.StatusCode != 0 {
		return fmt.Errorf("feishu error code=%d statusCode=%d msg=%s statusMessage=%s", feishuResp.Code, feishuResp.StatusCode, feishuResp.Msg, feishuResp.StatusMessage)
	}

	return nil
}

// truncateRunes 截断字符串到指定的 rune 长度, 过长时添加省略号
func truncateRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= 3 {
		return string(runes[:limit])
	}
	return string(runes[:limit-3]) + "..."
}
