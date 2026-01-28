package mysql

import (
	"log"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Init(dsn string) (*gorm.DB, error) {
	log.Println("connecting MySQL ... ")
	ret, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				LogLevel: logger.Warn,
				Colorful: false,
			},
		),
	})
	if err != nil {
		return nil, err
	}
	log.Println("connected")

	return ret, nil
}
