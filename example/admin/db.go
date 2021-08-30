package admin

import (
	"os"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/qor/media/media_library"
	"github.com/qor/qor5/example/models"
)

func ConnectDB() (db *gorm.DB) {
	var err error
	db, err = gorm.Open("postgres", os.Getenv("DB_PARAMS"))
	if err != nil {
		panic(err)
	}
	db.LogMode(true)
	err = db.AutoMigrate(
		&models.Post{},
		&media_library.MediaLibrary{},
	).Error
	if err != nil {
		panic(err)
	}
	return
}