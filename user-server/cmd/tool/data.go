//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

func main() {
	// 从 https://chunman4.com/forum.php?mod=viewthread&tid=66138&extra=page%3D1%26filter%3Dsortid%26sortid%3D1&page=1 获取网页文档
	resp, err := http.Get("https://chunman4.com/forum.php?mod=viewthread&tid=66138&extra=page%3D1%26filter%3Dsortid%26sortid%3D1&page=1")
	if err != nil {
		fmt.Println("获取网页失败:", err)
		return
	}
	defer resp.Body.Close()

	// 读取网页内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("读取网页内容失败:", err)
		return
	}

	// 打印网页内容
	fmt.Println(string(body))

	url := "https://api.gpt.ge/v1/chat/completions"
	method := "POST"

	// c传入 html 要求返回 可能的联系方式 qq 微信 电报 手机号 邮箱
	payload := strings.NewReader(`{
    "model": "gpt-4o",
    "messages": [
        {
            "role": "user",
            "content": "传入的html内容是：" + string(body)
        },
		{
			"role": "assistant",
			"content": "请返回可能的联系方式 qq 微信 电报 手机号 邮箱"
		}
    ],
    "max_tokens": 1688,
    "temperature": 0.5,
    "stream": false
}`)
	//设置请求头
	client := &http.Client{}

	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		fmt.Println(err)
		return
	}
	//设置请求头
	req.Header.Set("Authorization", "Bearer sk-3EBSt3SyLqjV2Sc7837b69CfB3044a7b913e3e5d0bE45153")
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer res.Body.Close()

	body, err = io.ReadAll(res.Body)

	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(string(body))

}
