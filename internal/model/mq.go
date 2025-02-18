package model

type MQ struct {
	ID    uint   `json:"id" gorm:"primary_key;column:id; type:int(10) unsigned; not null; auto_increment; comment:消息队列id"`
	Sid   string `json:"sid" gorm:"column:sid; type:string; comment:学号"`
	Bid   string `json:"bid" gorm:"column:bid; type:string; comment:绑定id"`
	Topic string `json:"topic" gorm:"column:topic; type:string; comment:消息主题"`
}
