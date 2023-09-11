package iat

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Iflytek struct {
	key    string
	secret string
	url    string
	appid  string
	status int
	done   chan struct{}
}

const (
	STATUS_FIRST_FRAME    = 0
	STATUS_CONTINUE_FRAME = 1
	STATUS_LAST_FRAME     = 2
)

func NewIflytek(key, secret, appid string) *Iflytek {
	return &Iflytek{
		key:    key,
		secret: secret,
		url:    "wss://iat-api.xfyun.cn/v2/iat",
		appid:  appid,
		status: STATUS_FIRST_FRAME,
		done:   make(chan struct{}),
	}
}

func (iat Iflytek) Connect() (*websocket.Conn, error) {
	ul, err := url.Parse(iat.url)
	if err != nil {
		return nil, err
	}
	date := time.Now().UTC().Format(time.RFC1123)
	//参与签名的字段 host ,date, request-line
	signString := []string{"host: " + ul.Host, "date: " + date, "GET " + ul.Path + " HTTP/1.1"}
	//拼接签名字符串
	sgin := strings.Join(signString, "\n")
	//签名结果
	sha := HmacWithShaTobase64("hmac-sha256", sgin, iat.secret)
	//构建请求参数 此时不需要urlencoding
	authUrl := fmt.Sprintf("hmac username=\"%s\", algorithm=\"%s\", headers=\"%s\", signature=\"%s\"", iat.key,
		"hmac-sha256", "host date request-line", sha)
	//将请求参数使用base64编码
	authorization := base64.StdEncoding.EncodeToString([]byte(authUrl))

	v := url.Values{}
	v.Add("host", ul.Host)
	v.Add("date", date)
	v.Add("authorization", authorization)
	//将编码后的字符串url encode后添加到url后面
	callurl := iat.url + "?" + v.Encode()
	d := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	conn, _, err := d.Dial(callurl, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func HmacWithShaTobase64(algorithm, data, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(data))
	encodeData := mac.Sum(nil)
	return base64.StdEncoding.EncodeToString(encodeData)
}

type RespData struct {
	Sid     string `json:"sid"`
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    Data   `json:"data"`
}

type Data struct {
	Result Result `json:"result"`
	Status int    `json:"status"`
}

type Decoder struct {
	results []*Result
}

func (d *Decoder) Decode(result *Result) {
	if len(d.results) <= result.Sn {
		d.results = append(d.results, make([]*Result, result.Sn-len(d.results)+1)...)
	}
	if result.Pgs == "rpl" {
		for i := result.Rg[0]; i <= result.Rg[1]; i++ {
			d.results[i] = nil
		}
	}
	d.results[result.Sn] = result
}

func (d *Decoder) String() string {
	var r string
	for _, v := range d.results {
		if v == nil {
			continue
		}
		r += v.String()
	}
	return r
}

type Result struct {
	Ls  bool   `json:"ls"`
	Rg  []int  `json:"rg"`
	Sn  int    `json:"sn"`
	Pgs string `json:"pgs"`
	Ws  []Ws   `json:"ws"`
}

func (t *Result) String() string {
	var wss string
	for _, v := range t.Ws {
		wss += v.String()
	}
	return wss
}

type Ws struct {
	Bg int  `json:"bg"`
	Cw []Cw `json:"cw"`
}

func (w *Ws) String() string {
	var wss string
	for _, v := range w.Cw {
		wss += v.W
	}
	return wss
}

type Cw struct {
	Sc int    `json:"sc"`
	W  string `json:"w"`
}

func (iat *Iflytek) Receive(conn *websocket.Conn, text chan<- string) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("iat receive err:", err)
			close(text)
			break
		}
		var resp = RespData{}
		if err := json.Unmarshal(message, &resp); err != nil {
			fmt.Println("unmarshal message err:", err)
			break
		}
		fmt.Println("iat receive:", resp.Data.Result.String(), resp.Sid)
		if resp.Code != 0 {
			fmt.Println(resp.Code, resp.Message)
			break
		}
		text <- resp.Data.Result.String()
		if resp.Data.Status == STATUS_LAST_FRAME {
			fmt.Println(resp.Code, resp.Message)
			close(text)
			break
		}
	}
}

func (iat *Iflytek) Send(conn *websocket.Conn, audio chan []byte) {
	for {
		select {
		case buffer := <-audio:
			fmt.Printf("iat send status:%d length:%d\n", iat.status, len(buffer))
			switch iat.status {
			case STATUS_FIRST_FRAME: //第一帧
				frameData := map[string]interface{}{
					"common": map[string]interface{}{
						"app_id": iat.appid,
					},
					"business": map[string]interface{}{
						"language": "zh_cn",
						"domain":   "iat",
						"accent":   "mandarin",
					},
					"data": map[string]interface{}{
						"status":   STATUS_FIRST_FRAME,
						"format":   "audio/L16;rate=16000",
						"audio":    base64.StdEncoding.EncodeToString(buffer),
						"encoding": "raw",
					},
				}
				conn.WriteJSON(frameData)
				iat.status = STATUS_CONTINUE_FRAME
			case STATUS_CONTINUE_FRAME: //后续帧
				frameData := map[string]interface{}{
					"data": map[string]interface{}{
						"status":   STATUS_CONTINUE_FRAME,
						"format":   "audio/L16;rate=16000",
						"audio":    base64.StdEncoding.EncodeToString(buffer),
						"encoding": "raw",
					},
				}
				conn.WriteJSON(frameData)
			}
		case <-iat.done:
			frameData := map[string]interface{}{
				"data": map[string]interface{}{
					"status":   STATUS_LAST_FRAME,
					"format":   "audio/L16;rate=16000",
					"audio":    base64.StdEncoding.EncodeToString([]byte{}),
					"encoding": "raw",
				},
			}
			conn.WriteJSON(frameData)
		}
	}
}

func (iat *Iflytek) Close() {
	close(iat.done)
}
