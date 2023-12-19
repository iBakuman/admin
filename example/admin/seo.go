package admin

import (
	"github.com/qor5/admin/example/models"
	"github.com/qor5/admin/presets"
	"github.com/qor5/admin/seo"
	"gorm.io/gorm"
	"net/http"
)

// @snippet_begin(SeoExample)
var seoBuilder *seo.Builder

func ConfigureSeo(pb *presets.Builder, db *gorm.DB, locales ...string) {
	seoBuilder = seo.NewBuilder(db, locales)
	seoBuilder.RegisterSEO("Post", &models.Post{}).RegisterContextVariable(
		"Title",
		func(object interface{}, _ *seo.Setting, _ *http.Request) string {
			if article, ok := object.(models.Post); ok {
				return article.Title
			}
			return ""
		},
	).RegisterSettingVariables("Test")
	seoBuilder.RegisterSEO("Product")
	seoBuilder.RegisterSEO("Announcement")
	seoBuilder.Configure(pb)
}

// @snippet_end
