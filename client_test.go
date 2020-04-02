package vivopush

import (
	"testing"
)

var (
	appId     = "your appId"
	appKey    = "your appId"
	appSecret = "your appSecret"
	regID1    = "your regID"
)

var msg1 = NewVivoMessage("hi baby1", "hi1")

func TestMiPush_Send(t *testing.T) {
	client, err := NewClient(appId, appKey, appSecret, 1)
	if err != nil {
		t.Errorf("TestMiPush_Send failed :%v\n", err)
	}
	result, err := client.Send(msg1, regID1)
	if err != nil {
		t.Errorf("TestMiPush_Send failed :%v\n", err)
	}
	t.Logf("result=%#v\n", result)
}

func TestMiPush_GetMessageStatusByJobKey(t *testing.T) {
	client, err := NewClient(appId, appKey, appSecret, 1)
	if err != nil {
		t.Errorf("TestMiPush_Send failed :%v\n", err)
	}
	result, err := client.GetMessageStatusByJobKey("jobId")
	if err != nil {
		t.Errorf("TestMiPush_GetMessageStatusByJobKey failed :%v\n", err)
	}
	t.Logf("result=%#v\n", result)
}
