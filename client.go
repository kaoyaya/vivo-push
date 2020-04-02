package vivopush

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/bitly/go-simplejson"
)

var authToken = new(AuthToken)

type VivoClient struct {
	AppId     string
	AppKey    string
	AppSecret string
}

type VivoTokenPar struct {
	AppId     string `json:"appId"`
	AppKey    string `json:"appKey"`
	Timestamp int64  `json:"timestamp"`
	Sign      string `json:"sign"`
}

type AuthToken struct {
	token     string
	validTime int64
}

type VivoPush struct {
	host      string
	AuthToken string

	client *VivoClient

	mu *sync.Mutex
}

func NewClient(appId, appKey, appSecret string, refreshInterval int) (*VivoPush, error) {
	vc := &VivoClient{
		appId,
		appKey,
		appSecret,
	}
	token, err := vc.GetToken()
	if err != nil {
		return nil, err
	}

	client := &VivoPush{
		host:      ProductionHost,
		AuthToken: token,
		client:    vc,
	}

	if refreshInterval <= 0 {
		refreshInterval = 300
	}

	go client.refreshToken(refreshInterval)

	return client, nil
}

func (v *VivoPush) refreshToken(interval int) {
	tick := time.Tick(time.Duration(interval) * time.Second)
	for {
		select {
		case <-tick:
			token, err := v.client.GetToken()
			if err != nil {
				log.Printf("force refresh vivo token err: %+v", err)
			}
			v.mu.Lock()
			v.AuthToken = token
			v.mu.Unlock()
			log.Printf("refresh vivo push token at: %s", time.Now().String())
		}
	}
}

// 获取token  返回的 expiretime 秒  当过期的时候
func (vc *VivoClient) GetToken() (string, error) {
	now := time.Now().UnixNano() / 1e6
	if authToken != nil {
		if authToken.validTime > now {
			return authToken.token, nil
		}
	}
	md5Ctx := md5.New()
	md5Ctx.Write([]byte(vc.AppId + vc.AppKey + strconv.FormatInt(now, 10) + vc.AppSecret))
	sign := hex.EncodeToString(md5Ctx.Sum(nil))

	formData, err := json.Marshal(&VivoTokenPar{
		AppId:     vc.AppId,
		AppKey:    vc.AppKey,
		Timestamp: now,
		Sign:      sign,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST", ProductionHost+AuthURL, bytes.NewReader(formData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	result, err := handleResponse(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("network error")
	}
	js, err := simplejson.NewJson(result)
	if err != nil {
		return "", err
	}
	token, err := js.Get("authToken").String()
	if err != nil {
		return "", err
	}
	// 1小时有效
	authToken.token = token
	authToken.validTime = now + 3600000
	return token, nil
}

//----------------------------------------Sender----------------------------------------//
// 根据regID，发送消息到指定设备上
func (v *VivoPush) Send(msg *Message, regID string) (*SendResult, error) {
	params := v.assembleSendParams(msg, regID)
	res, err := v.doPost(v.host+SendURL, params)
	if err != nil {
		return nil, err
	}
	var result SendResult
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}
	if result.Result != 0 {
		return nil, errors.New(result.Desc)
	}
	return &result, nil
}

// 保存群推消息公共体接口
func (v *VivoPush) SaveListPayload(msg *MessagePayload) (*SendResult, error) {
	res, err := v.doPost(v.host+SaveListPayloadURL, msg.JSON())
	if err != nil {
		return nil, err
	}
	var result SendResult
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}
	if result.Result != 0 {
		return nil, errors.New(result.Desc)
	}
	return &result, nil
}

// 群推
func (v *VivoPush) SendList(msg *MessagePayload, regIds []string) (*SendResult, error) {
	if len(regIds) < 2 || len(regIds) > 1000 {
		return nil, errors.New("regIds个数必须大于等于2,小于等于 1000")
	}
	res, err := v.SaveListPayload(msg)
	if err != nil {
		return nil, err
	}
	if res.Result != 0 {
		return nil, errors.New(res.Desc)
	}
	bytesData, err := json.Marshal(NewListMessage(regIds, res.TaskId))
	if err != nil {
		return nil, err
	}
	//推送
	res2, err := v.doPost(v.host+PushToListURL, bytesData)
	if err != nil {
		return nil, err
	}
	var result SendResult
	err = json.Unmarshal(res2, &result)
	if err != nil {
		return nil, err
	}
	if result.Result != 0 {
		return nil, errors.New(result.Desc)
	}
	return &result, nil
}

// 全量推送
func (v *VivoPush) SendAll(msg *MessagePayload) (*SendResult, error) {
	res2, err := v.doPost(v.host+PushToAllURL, msg.JSON())
	if err != nil {
		return nil, err
	}
	var result SendResult
	err = json.Unmarshal(res2, &result)
	if err != nil {
		return nil, err
	}
	if result.Result != 0 {
		return nil, errors.New(result.Desc)
	}
	return &result, nil
}

//----------------------------------------Tracer----------------------------------------//
// 获取指定消息的状态。
func (v *VivoPush) GetMessageStatusByJobKey(jobKey string) (*BatchStatusResult, error) {
	params := v.assembleStatusByJobKeyParams(jobKey)
	res, err := v.doGet(v.host+MessagesStatusURL, params)
	if err != nil {
		return nil, err
	}
	var result BatchStatusResult
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (v *VivoPush) assembleSendParams(msg *Message, regID string) []byte {
	msg.RegId = regID
	jsondata := msg.JSON()
	return jsondata
}

func (v *VivoPush) assembleStatusByJobKeyParams(jobKey string) string {
	form := url.Values{}
	form.Add("taskIds", jobKey)
	return "?" + form.Encode()
}

func handleResponse(response *http.Response) ([]byte, error) {
	defer func() {
		_ = response.Body.Close()
	}()
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (v *VivoPush) doPost(url string, formData []byte) ([]byte, error) {
	var result []byte
	var req *http.Request
	var resp *http.Response
	var err error

	req, err = http.NewRequest("POST", url, bytes.NewReader(formData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authToken", v.AuthToken)
	client := &http.Client{}

	// TODO:@klaus 这里重试逻辑写得不好
	// 需要抽出一个专门的 client 管理  http 请求
	tryTime := 0
tryAgain:
	resp, err = client.Do(req)
	if err != nil {
		tryTime += 1
		if tryTime < _postRetryTimes {
			goto tryAgain
		}
		return nil, err
	}
	result, err = handleResponse(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("network error")
	}
	return result, nil
}

func (v *VivoPush) doGet(url string, params string) ([]byte, error) {
	var (
		result []byte
		req    *http.Request
		resp   *http.Response
		err    error
	)
	req, err = http.NewRequest("GET", url+params, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authToken", v.AuthToken)

	// TODO:@klaus 重用 client
	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		return nil, err
	}
	result, err = handleResponse(resp)
	if err != nil {
		return nil, err
	}
	return result, nil
}
