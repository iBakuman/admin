package seo

import (
	"strings"

	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var dbForTest *gorm.DB

func init() {
	// if db, err := gorm.Open(postgres.Open(os.Getenv("DBURL")), &gorm.Config{}); err != nil {
	if db, err := gorm.Open(postgres.Open("user=blog password=123 dbname=blog_dev sslmode=disable host=localhost port=7890"), &gorm.Config{}); err != nil {
		panic(err)
	} else {
		dbForTest = db
	}
}

// @snippet_begin(SeoModelExample)
type Product struct {
	Name string
	SEO  Setting
}

// @snippet_end

func resetDB() {
	dbForTest.Exec("truncate qor_seo_settings;")
}

func metaEqual(got, want string) bool {
	for _, s := range strings.Split(want, "\n") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(got, s) {
			return false
		}
	}
	return true
}
