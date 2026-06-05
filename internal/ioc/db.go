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
		DisableForeignKeyConstraintWhenMigrating: true,
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
		&model.ActivitySigner{},
		&model.Comment{},
		&model.Post{},
		&model.PostDraft{},
		&model.Approvement{},
		&model.AuditorForm{},
		&model.Image{},
		&model.UserActivityInteraction{},
		&model.UserPostInteraction{},
		&model.UserCommentInteraction{},
	); err != nil {
		return err
	}

	return db.AutoMigrate(&model.Feed{})
}
