package main

import (
	"fmt"
	"io"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"
	"math/rand"
	"net/http"
	"strings"
	"time"
	"unicode"
)

func main() {
	db.InitDB()
	// 读取smlist 所有列表并循环
	smlistServer := service.NewSmlistService()
	smsList, total, err := smlistServer.GetSmlistAllList()
	if err != nil {
		fmt.Println(err)
		return
	}
	if total == 0 {
		fmt.Println("没有数据")
		return
	}

	for _, smsList := range smsList {
		source_id := smsList.ID
		name := smsList.Name
		city := smsList.City
		desc := smsList.Desc
		address := smsList.Address
		qq := smsList.QQ

		// 检查是否是qq
		qq_list := formateQq(qq, source_id, name, city, address, desc)
		if len(qq_list) > 0 {
			// 保存到线索库
			clueServer := service.NewClueService()
			if err := clueServer.BatchSaveClue(qq_list); err != nil {
				fmt.Println(err)
			}
		}

		// 检查是否是微信
		wx := smsList.WX
		wx_list := formateWx(wx, source_id, name, city, address, desc)
		if len(wx_list) > 0 {
			// 保存到线索库
			clueServer := service.NewClueService()
			if err := clueServer.BatchSaveClue(wx_list); err != nil {
				fmt.Println(err)
			}
		}

		// 检查是否是手机号
		phone := smsList.Phone
		phone_list := formatePhone(phone, source_id, name, city, address, desc)
		if len(phone_list) > 0 {
			// 保存到线索库
			clueServer := service.NewClueService()
			if err := clueServer.BatchSaveClue(phone_list); err != nil {
				fmt.Println(err)
			}
		}

		// 检查是否是电报
		telegram := smsList.Tg
		telegram_list := formateTelegram(telegram, source_id, name, city, address, desc)
		if len(telegram_list) > 0 {
			// 保存到线索库
			clueServer := service.NewClueService()
			if err := clueServer.BatchSaveClue(telegram_list); err != nil {
				fmt.Println(err)
			}
		}

		// 检查是否推特
		x := smsList.X
		twitter_list := formateTwitter(x, source_id, name, city, address, desc)
		if len(twitter_list) > 0 {
			// 保存到线索库
			clueServer := service.NewClueService()
			if err := clueServer.BatchSaveClue(twitter_list); err != nil {
				fmt.Println(err)
			}
		}

	}
}

func formateTwitter(twitter string, source_id string, name string, city string, address string, desc string) []*model.Clue {
	if len(twitter) == 0 {
		return []*model.Clue{}
	}
	// 判断有中文逗号替换为英文逗号
	if strings.Contains(twitter, "，") {
		twitter = strings.Replace(twitter, "，", ",", -1)
	}
	// 判断有英文逗号 截断
	tem_list := []string{}
	if strings.Contains(twitter, ",") {
		tem_list = strings.Split(twitter, ",")
	} else {
		tem_list = append(tem_list, twitter)
	}

	// 遍历tem_list 去掉空格
	twitter_list := []*model.Clue{}
	for _, new_twitter := range tem_list {
		// 为空继续
		if len(new_twitter) == 0 {
			continue
		}
		// 判断是否包含任意汉字
		is_has := HasChinese(new_twitter)

		if !is_has {
			// 构造对象
			clue := model.Clue{
				SourceID: source_id,
				Name:     name,
				City:     city,
				Desc:     desc,
				Address:  address,
				Account:  new_twitter,
				Type:     6,
			}
			// 保存到数组
			twitter_list = append(twitter_list, &clue)
		}
	}
	return twitter_list
}

func formateTelegram(telegram string, source_id string, name string, city string, address string, desc string) []*model.Clue {
	if len(telegram) == 0 {
		return []*model.Clue{}
	}
	// 判断有中文逗号替换为英文逗号
	if strings.Contains(telegram, "，") {
		telegram = strings.Replace(telegram, "，", ",", -1)
	}
	// 判断有英文逗号 截断
	tem_list := []string{}
	if strings.Contains(telegram, ",") {
		tem_list = strings.Split(telegram, ",")
	} else {
		tem_list = append(tem_list, telegram)
	}

	// 遍历tem_list 去掉空格
	telegram_list := []*model.Clue{}
	for _, new_telegram := range tem_list {
		// 为空继续
		if len(new_telegram) == 0 {
			continue
		}
		// 判断是否包含任意汉字
		is_has := HasChinese(new_telegram)

		if !is_has {
			// 构造对象
			clue := model.Clue{
				SourceID: source_id,
				Name:     name,
				City:     city,
				Desc:     desc,
				Address:  address,
				Account:  new_telegram,
				Type:     4,
			}
			// 保存到数组
			telegram_list = append(telegram_list, &clue)
		}
	}
	return telegram_list
}

func formatePhone(phone string, source_id string, name string, city string, address string, desc string) []*model.Clue {
	if len(phone) == 0 {
		return []*model.Clue{}
	}
	// 判断有中文逗号替换为英文逗号
	if strings.Contains(phone, "，") {
		phone = strings.Replace(phone, "，", ",", -1)
	}
	// 判断有英文逗号 截断
	tem_list := []string{}
	if strings.Contains(phone, ",") {
		tem_list = strings.Split(phone, ",")
	} else {
		tem_list = append(tem_list, phone)
	}

	// 遍历tem_list 去掉空格
	phone_list := []*model.Clue{}
	for _, new_phone := range tem_list {
		// 为空继续
		if len(new_phone) == 0 {
			continue
		}
		if len(new_phone) != 11 {
			continue
		}
		// 判断是否包含任意汉字
		is_has := HasChinese(new_phone)

		if !is_has {
			// 构造对象
			clue := model.Clue{
				SourceID: source_id,
				Name:     name,
				City:     city,
				Desc:     desc,
				Address:  address,
				Account:  new_phone,
				Type:     3,
			}
			// 保存到数组
			phone_list = append(phone_list, &clue)
		}
	}
	return phone_list
}

func HasChinese(str string) bool {
	for _, r := range str {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
func formateWx(wx string, source_id string, name string, city string, address string, desc string) []*model.Clue {
	if len(wx) == 0 {
		return []*model.Clue{}
	}
	// 判断有中文逗号替换为英文逗号
	if strings.Contains(wx, "，") {
		wx = strings.Replace(wx, "，", ",", -1)
	}
	// 判断有英文逗号 截断
	tem_list := []string{}
	if strings.Contains(wx, ",") {
		tem_list = strings.Split(wx, ",")
	} else {
		tem_list = append(tem_list, wx)
	}

	// 遍历tem_list 去掉空格
	wx_list := []*model.Clue{}
	for _, new_wx := range tem_list {
		// 为空继续
		if len(new_wx) == 0 {
			continue
		}
		// 判断是否包含任意汉字
		is_has := HasChinese(new_wx)

		if !is_has {
			// 构造对象
			clue := model.Clue{
				SourceID: source_id,
				Name:     name,
				City:     city,
				Desc:     desc,
				Address:  address,
				Account:  new_wx,
				Type:     2,
			}
			// 保存到数组
			wx_list = append(wx_list, &clue)
		}
	}
	return wx_list
}

func formateQq(qq string, source_id string, name string, city string, address string, desc string) []*model.Clue {
	if len(qq) == 0 {
		return []*model.Clue{}
	}
	// 判断有中文逗号替换为英文逗号
	if strings.Contains(qq, "，") {
		qq = strings.Replace(qq, "，", ",", -1)
	}
	// 判断有英文逗号 截断
	tem_list := []string{}
	if strings.Contains(qq, ",") {
		tem_list = strings.Split(qq, ",")
	} else {
		tem_list = append(tem_list, qq)
	}

	// 遍历tem_list 去掉空格
	qq_list := []*model.Clue{}
	for _, new_qq := range tem_list {
		if len(new_qq) == 0 {
			continue
		}
		if len(new_qq) > 11 {
			continue
		}
		// 随机休眠1-3 秒
		time.Sleep(time.Duration(rand.Intn(2)+1) * time.Second)
		is_qq := check_qq(new_qq)
		if is_qq {
			// 构造对象
			clue := model.Clue{
				SourceID: source_id,
				Name:     name,
				City:     city,
				Desc:     desc,
				Address:  address,
				Account:  new_qq,
				Type:     1,
			}
			// 保存到数组
			qq_list = append(qq_list, &clue)
		}
	}
	return qq_list
}

func check_qq(qq string) bool {
	url := "https://user.qzone.qq.com/" + qq + "/infocenter"
	//  request访问url
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return false
	}
	// 打印body
	// fmt.Println(string(body))
	if strings.Contains(string(body), "404") {
		return false
	}
	return true
}
