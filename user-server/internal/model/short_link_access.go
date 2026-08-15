package model

import (
	"time"
)

// ShortLinkAccess 短链访问统计模型
type ShortLinkAccess struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ShortLinkID uint      `json:"short_link_id" gorm:"not null;index;comment:短链ID"` 
	IP          string    `json:"ip" gorm:"not null;comment:IP地址"`                  
	UserAgent   string    `json:"user_agent" gorm:"comment:用户代理"`                   
	Referer     string    `json:"referer" gorm:"comment:来源页面"`                      
	DeviceType  string    `json:"device_type" gorm:"comment:设备类型"`                  
	Browser     string    `json:"browser" gorm:"comment:浏览器"`                       
	OS          string    `json:"os" gorm:"comment:操作系统"`                           
	Location    string    `json:"location" gorm:"comment:地理位置"`                     
	AccessTime  time.Time `json:"access_time" gorm:"autoCreateTime;comment:访问时间"`   
}

// TableName 返回表名
func (ShortLinkAccess) TableName() string {
	return "short_link_accesses"
}

