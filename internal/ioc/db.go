package ioc

import (
	"log"

	_ "github.com/go-sql-driver/mysql"
	"github.com/raiki02/EG/config"
	"github.com/raiki02/EG/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func InitDB(cfg *config.Conf) *gorm.DB {
	db, err := gorm.Open(mysql.Open(cfg.Mysql.DSN), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})
	if err != nil {
		log.Fatalln(err)
	}
	sqldb, err := db.DB()
	if err != nil {
		log.Fatalln(err)
	}
	sqldb.SetMaxIdleConns(cfg.Mysql.MaxIdleConns)
	sqldb.SetMaxOpenConns(cfg.Mysql.MaxOpenConns)

	err = migrate(db)
	if err != nil {
		log.Fatalln(err)
	}

	return db
}

func migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.User{},
		&model.Activity{},
		&model.ActivityDraft{},
		&model.Comment{},
		&model.Post{},
		&model.PostDraft{},
		&model.Approvement{},
		&model.AuditorForm{},
	); err != nil {
		return err
	}

	if db.Migrator().HasTable(&model.Feed{}) {
		if err := db.Exec(`
DELETE f1 FROM feed AS f1
INNER JOIN feed AS f2
ON f1.receiver = f2.receiver
AND f1.student_id = f2.student_id
AND f1.action = f2.action
AND f1.object = f2.object
AND f1.target_bid = f2.target_bid
AND f1.id > f2.id
`).Error; err != nil {
			return err
		}
	}

	return db.AutoMigrate(&model.Feed{})
}
