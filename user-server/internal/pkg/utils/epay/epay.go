package epay

import (
	"crypto/md5"
	"errors"
	"fmt"
	"marketing/internal/pkg/utils/common"
	_type "marketing/internal/pkg/utils/type"
	"net/url"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

func EpayUrl(orderID string, price decimal.Decimal, productName string, epayConfig _type.EpayConfig) string {
	submitData := map[string]string{
		"pid":          epayConfig.Pid,
		"type":         epayConfig.Type,
		"out_trade_no": orderID,
		"notify_url":   epayConfig.NotifyUrl,
		"return_url":   epayConfig.ReturnUrl,
		"name":         productName,
		"money":        price.String(),
	}
	submitData["sign"] = EpaySign(submitData, epayConfig.Key)
	submitData["sign_type"] = "MD5"

	//生成url
	values := url.Values{}
	for key, value := range submitData {
		values.Add(key, value)
	}
	payUrl := fmt.Sprintf("%s?%s", epayConfig.EpayUrl, values.Encode())
	return payUrl
}

func EpaySign(mapInput map[string]string, epayKey string) string {
	//排序key获取排序后的key列表
	var keys []string
	for k := range mapInput {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queryParts []string
	for _, key := range keys {
		key := key
		value := mapInput[key]
		if value == "" || key == "sign" || key == "sign_type" {
			continue
		}
		queryParts = append(queryParts, key+"="+value)
	}

	stringToSign := strings.Join(queryParts, "&")
	stringToSign = stringToSign + epayKey
	stringToSign, _ = url.QueryUnescape(stringToSign)

	//md5
	inputBytes := []byte(stringToSign)
	md5Hash := md5.Sum(inputBytes)
	md5String := fmt.Sprintf("%x", md5Hash)

	return md5String
}

func EpayQuery(orderID string, epayConfig _type.EpayConfig) (bool, error) {

	var pid = epayConfig.Pid
	var key = epayConfig.Key
	var query_url = epayConfig.QueryUrl

	var url = fmt.Sprintf("%s?act=order&pid=%s&key=%s&out_trade_no=%s", query_url, pid, key, orderID)

	var res, err = common.GetRequest(url)
	if err != nil {
		return false, err
	}
	var code = res["code"].(float64)
	if code != 1 {
		return false, errors.New("订单不存在")
	}
	return true, nil

}
