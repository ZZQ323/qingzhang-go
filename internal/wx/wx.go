package wx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Session struct {
	Openid     string `json:"openid"`
	SessionKey string `json:"session_key"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

var client = &http.Client{Timeout: 5 * time.Second}

// Code2Session 用小程序登录 code 换取 openid
func Code2Session(appID, secret, code string) (*Session, error) {
	u := "https://api.weixin.qq.com/sns/jscode2session?" + url.Values{
		"appid":      {appID},
		"secret":     {secret},
		"js_code":    {code},
		"grant_type": {"authorization_code"},
	}.Encode()

	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var s Session
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	if s.Openid == "" {
		return nil, fmt.Errorf("wx login failed: errcode=%d msg=%s", s.ErrCode, s.ErrMsg)
	}
	return &s, nil
}
